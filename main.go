package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/joho/godotenv"
)

type Configuration struct {
	Endpoint    string
	ApiKey      string
	Prefix      string
	Section     string
	Title       string
	CategoryURI string
	Branch      string
	BaseURL     string
	Pages       PagesConfiguration
}

type PagesConfiguration struct {
	MarkdownFolder string
	PagesFile      string
	OldPages       []string
	NewPages       []string
}

type ReadmeIoPage struct {
	Slug     string `json:"slug,omitempty"`
	Title    string `json:"title,omitempty"`
	Body     string `json:"body,omitempty"`
	Category string `json:"category,omitempty"`
	Parent   string `json:"parent,omitempty"`
}

type ReadmeIoPageContent struct {
	Body string `json:"body,omitempty"`
}

type ReadmeIoPageUpdate struct {
	Title   string               `json:"title,omitempty"`
	Content *ReadmeIoPageContent `json:"content,omitempty"`
}

// Reference types for create payloads that expect objects with a URI
type ReadmeIoReference struct {
	URI string `json:"uri,omitempty"`
}

type ReadmeIoPageCreate struct {
	Slug     string             `json:"slug,omitempty"`
	Title    string             `json:"title,omitempty"`
	Category *ReadmeIoReference `json:"category,omitempty"`
	Parent   *ReadmeIoReference `json:"parent,omitempty"`
}

type Pages struct {
	Pages []Page
}

type Page struct {
	ParentSlug string
	PageSlug   string
	Title      string
	Content    string
}

type Item struct {
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	Item        []Item     `json:"item,omitempty"`
	Request     Request    `json:"request,omitempty"`
	Response    []Response `json:"response,omitempty"`
}

type Request struct {
	Method      string   `json:"method"`
	URL         RawURL   `json:"url"`
	Body        Body     `json:"body"`
	Header      []Header `json:"header"`
	Description string   `json:"description,omitempty"`
	Auth        Auth     `json:"auth,omitempty"`
}

type Response struct {
	Name    string  `json:"name"`
	Body    string  `json:"body"`
	Request Request `json:"originalRequest,omitempty"`
}

// RawURL can hold either a URL string or a URL object with a "raw" property.
type RawURL struct {
	URLString string
	URLObject *URLObject
}

// URLObject represents the structure of the URL when it's not a simple string.
type URLObject struct {
	Raw string `json:"raw"`
}

type Auth struct {
	Type string `json:"type,omitempty"`
}

type Body struct {
	Raw string `json:"raw"`
}

type Header struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// UnmarshalJSON custom unmarshaler for RawURL to handle both string and object.
func (r *RawURL) UnmarshalJSON(data []byte) error {
	// First, try to unmarshal as a string.
	var urlString string
	if err := json.Unmarshal(data, &urlString); err == nil {
		r.URLString = urlString
		return nil
	}

	// If not a string, try to unmarshal as an object.
	var urlObject URLObject
	if err := json.Unmarshal(data, &urlObject); err == nil {
		r.URLObject = &urlObject
		return nil
	}

	return errors.New("url field is neither a string nor a recognized object")
}

var configuration Configuration
var pages Pages

func main() {
	// Attempt to load .env file if it exists
	if _, err := os.Stat(".env"); err == nil {
		if err := godotenv.Load(); err != nil {
			panic(fmt.Sprintf("Error loading .env file: %s", err))
		}
	} else {
		fmt.Println("No .env file found. Proceeding with environment variables or default values.")
	}

	sourceFile := os.Getenv("COLLECTION_SOURCE_FILE")
	if sourceFile == "" {
		panic("Error: COLLECTION_SOURCE_FILE is required")
	}
	file, err := os.ReadFile(sourceFile)
	if err != nil {
		panic(fmt.Sprintf("Error reading Postman collection: %s", err))
	}

	configuration.Pages.MarkdownFolder = os.Getenv("MARKDOWN_FOLDER")
	if configuration.Pages.MarkdownFolder == "" {
		panic("Error: MARKDOWN_FOLDER is required")
	}
	// Create docs directory if not exists
	err = os.MkdirAll(configuration.Pages.MarkdownFolder, os.ModePerm)
	if err != nil {
		panic(fmt.Sprintf("Error creating docs directory: %s", err))
	}

	configuration.Section = os.Getenv("README_API_SECTION")
	if configuration.Section == "" {
		panic("Error: README_API_SECTION is required")
	}
	configuration.Section = strings.ToLower(strings.TrimSpace(configuration.Section))
	if configuration.Section != "reference" && configuration.Section != "guides" {
		panic("Error: README_API_SECTION must be 'reference' or 'guides'")
	}

	configuration.Title = os.Getenv("README_API_CATEGORY_TITLE")
	if configuration.Title == "" {
		panic("Error: README_API_CATEGORY_TITLE is required")
	}

	configuration.BaseURL = os.Getenv("COLLECTION_BASE_URL")

	configuration.Prefix = os.Getenv("README_API_PREFIX")
	if configuration.Prefix == "" {
		panic("Error: README_API_PREFIX is required")
	}

	var postmanCollection struct {
		Item []Item `json:"item"`
	}

	err = json.Unmarshal(file, &postmanCollection)
	if err != nil {
		panic(fmt.Sprintf("Error unmarshalling Postman collection: %s", err))
	}

	// Generate markdown pages
	for _, item := range postmanCollection.Item {
		createRootPage(item, configuration.Pages.MarkdownFolder)
	}

	configuration.Endpoint = os.Getenv("README_API_ENDPOINT")
	if configuration.Endpoint == "" {
		fmt.Println("Markdown generated. Publish process stopped: README_API_ENDPOINT is empty")
		return
	}
	configuration.ApiKey = os.Getenv("README_API_KEY")
	if configuration.ApiKey == "" {
		fmt.Println("Markdown generated. Publish process stopped: README_API_KEY is empty")
		return
	}
	configuration.Branch = os.Getenv("README_API_BRANCH")
	if configuration.Branch == "" {
		fmt.Println("Markdown generated. Publish process stopped: README_API_BRANCH is empty")
		return
	}

	// Resolve category URI once and reuse for all pages
	configuration.CategoryURI = getCategoryURI(configuration.Title)
	if configuration.CategoryURI == "" {
		fmt.Printf("Markdown generated. Publish process stopped: category URI not found for title '%s'\n", configuration.Title)
		return
	}
	configuration.Pages.PagesFile = os.Getenv("README_API_CREATED_PAGES_FILE")
	if configuration.Pages.PagesFile == "" {
		fmt.Println("Markdown generated. Publish process stopped: README_API_CREATED_PAGES_FILE is empty")
		return
	}
	configuration.Pages.OldPages = loadPreviouslyCreatedPages(configuration.Pages.PagesFile)

	// Update or create pages in Readme.io
	for _, item := range pages.Pages {
		if item.ParentSlug == "" {
			upsertPage("", item.PageSlug, item.Title, item.Content)
		}
	}
	for _, item := range pages.Pages {
		if item.ParentSlug != "" {
			upsertPage(item.ParentSlug, item.PageSlug, item.Title, item.Content)
		}
	}

	// todo compare old and new pages and delete the ones that are not in the new list
	if len(configuration.Pages.OldPages) == 0 {
		if updatePagesList(configuration.Pages.PagesFile, configuration.Pages.NewPages) {
			return
		}
	}

	// Create a map to store items of the NewPages array
	itemsMap := make(map[string]bool)
	for _, item := range configuration.Pages.NewPages {
		itemsMap[item] = true
	}

	// Find items in OldPages that do not exist in NewPages array
	var diff []string
	for _, item := range configuration.Pages.OldPages {
		if !itemsMap[item] {
			diff = append(diff, item)
		}
	}

	//Order the diff array in a reverse order to remove the child pages first
	sort.Sort(sort.Reverse(sort.StringSlice(diff)))

	// Delete pages that are not in the new list
	var isDeleted = true
	for _, item := range diff {
		if deletePage(item) {
			fmt.Printf("Delete the page %s\n", item)
		} else {
			isDeleted = false
		}
	}

	updatePagesList(configuration.Pages.PagesFile, configuration.Pages.NewPages)

	if !isDeleted {
		panic("Error deleting pages")
	}
}

func updatePagesList(filepath string, lines []string) bool {
	file, err := os.Create(filepath)
	if err != nil {
		fmt.Println("Error creating file:", err)
		return true
	}
	defer file.Close()

	// Create a new writer
	writer := bufio.NewWriter(file)

	// Write each line to the file
	for _, line := range lines {
		_, err := writer.WriteString(line + "\n")
		if err != nil {
			panic(fmt.Sprintf("Error writing to file: %v", err))
		}
	}

	// Flush the writer to ensure all data is written to the file
	err = writer.Flush()
	if err != nil {
		panic(fmt.Sprintf("Error flushing writer: %v", err))
	}
	return false
}

func loadPreviouslyCreatedPages(file string) []string {
	createdPagesFile, err := os.Open(file)
	if err != nil {
		panic(fmt.Sprintf("Error opening file: %v", err))
	}
	defer createdPagesFile.Close()

	// Create a new scanner
	scanner := bufio.NewScanner(createdPagesFile)

	// Create a slice to hold the lines
	var createdPages []string

	// Read the file line by line and append to the slice
	for scanner.Scan() {
		line := cleanString(scanner.Text())
		if line != "" {
			createdPages = append(createdPages, line)
		}
	}

	// Check for errors during scanning
	if err := scanner.Err(); err != nil {
		fmt.Println("Error reading file:", err)
		panic(err)
	}

	return createdPages
}

func createRootPage(item Item, destinationFolder string) {
	slug := fmt.Sprintf("%s-%s", configuration.Prefix, generateSlug(item.Name))
	header := ""
	description := ""
	hasContent := false
	if item.Description != "" {
		description = fmt.Sprintf("\n%s\n\n", escapePreservingFences(cleanString(item.Description)))
		hasContent = true
	}

	content := header + description + "\n"
	if isQuery(item) {
		content += processQuery(item)
		hasContent = true
	}

	hasList := false
	listContent := ""
	for _, subItem := range item.Item {
		if isQuery(subItem) {
			content += fmt.Sprintf("\n%s", processQuery(subItem))
			hasContent = true
		} else {
			subPageSlug := fmt.Sprintf("%s-%s", slug, generateSlug(subItem.Name))
			subFolderLink := fmt.Sprintf("[%s](/%s/%s)\n", escapeOutsideInlineCode(cleanString(subItem.Name)), configuration.Section, subPageSlug)
			listContent += fmt.Sprintf("- %s", subFolderLink)
			createSubPage(slug, subItem, subPageSlug, destinationFolder, "")
			hasList = true
		}
	}
	if hasList {
		if hasContent {
			content += "\n***\n"
		}
		content += "\n# Subsections\n"
		content += listContent
	}

	// Write content to file
	filename := fmt.Sprintf("%s/%s.md", destinationFolder, slug)
	err := os.WriteFile(filename, []byte(content), os.ModePerm)
	if err != nil {
		fmt.Println("Error writing file:", filename, err)
	}

	pages.Pages = append(pages.Pages, Page{
		PageSlug: slug,
		Title:    cleanString(item.Name),
		Content:  content,
	})
}

func createSubPage(parentSlug string, item Item, slug string, destinationFolder string, level string) {
	content := processSubItem(item, level)

	// Write content to file
	filename := fmt.Sprintf("%s/%s.md", destinationFolder, slug)
	err := os.WriteFile(filename, []byte(content), os.ModePerm)
	if err != nil {
		fmt.Println("Error writing file:", filename, err)
	}

	pages.Pages = append(pages.Pages, Page{
		ParentSlug: parentSlug,
		PageSlug:   slug,
		Title:      cleanString(item.Name),
		Content:    content,
	})
}

func processQuery(item Item) string {
	content := getQueryHeader(item)
	if item.Request.Description != "" {
		content += fmt.Sprintf("\n%s\n", escapePreservingFences(cleanString(item.Request.Description)))
	}
	url := getRequestURL(item.Request)
	headers := getRequestHeaders(item.Request)
	requestExample := fmt.Sprintf("\n```json js%s%s\n%s\n```\n\n", url, headers, cleanString(item.Request.Body.Raw))
	responseExamples := ""
	if len(item.Response) > 0 {
		for _, response := range item.Response {
			responseExamples += fmt.Sprintf("\n**Example: %s**\n", escapeOutsideInlineCode(cleanString(response.Name)))
			responseExamples += fmt.Sprintf("\n```json js\n// Request →%s%s\n%s\n```\n", getRequestURL(response.Request), getRequestHeaders(response.Request), cleanString(response.Request.Body.Raw))
			responseExamples += fmt.Sprintf("\n```json js\n// Response ←\n%s\n```\n\n", cleanString(response.Body))
		}
	}
	return content + requestExample + responseExamples
}

func getRequestURL(request Request) string {
	url := ""
	if request.URL.URLString != "" {
		url = request.URL.URLString
	} else if request.URL.URLObject != nil {
		url = request.URL.URLObject.Raw
	}
	url = strings.ReplaceAll(url, "{{BaseURL}}", configuration.BaseURL)
	return fmt.Sprintf("\n// %s %s", request.Method, cleanString(url))
}

func getRequestHeaders(request Request) string {
	authHeaders := "Authorization: Bearer <token>"
	if request.Auth.Type == "noauth" {
		authHeaders = ""
	}
	headers := ""
	if authHeaders != "" {
		headers = fmt.Sprintf("\n// %s", authHeaders)
	}
	return headers
}

func processSubItem(item Item, level string) string {
	content := ""
	if isQuery(item) {
		content += fmt.Sprintf("\n%s", processQuery(item))
	} else if len(item.Item) > 0 {
		content += getHeader(level, item)
		if item.Description != "" {
			content += fmt.Sprintf("\n%s\n", escapePreservingFences(cleanString(item.Description)))
		}
		for _, subitem := range item.Item {
			content += processSubItem(subitem, fmt.Sprintf("%s#", level))
		}
	} else {
		content += fmt.Sprintf("\n%s", escapePreservingFences(cleanString(item.Description)))
	}
	return content
}

func upsertPage(parentSlug string, slug string, title string, content string) {
	configuration.Pages.NewPages = append(configuration.Pages.NewPages, slug)
	pageExists := checkPageExists(slug)
	if pageExists {
		fmt.Printf("Update the page %s\n", slug)
		updatePage(slug, title, content)
	} else {
		fmt.Printf("Create the page %s\n", slug)
		createPage(parentSlug, slug, title, content)
	}
}

func checkPageExists(slug string) bool {
	resp := sendRequest("GET", slug, nil)
	if resp == nil {
		return false
	}
	ok := resp.StatusCode == http.StatusOK
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return ok
}

func createPage(parentSlug string, slug string, title string, content string) bool {
	createdBody := ReadmeIoPageCreate{
		Slug:  slug,
		Title: title,
	}
	// Add category (resolved once) if available
	if configuration.CategoryURI != "" {
		createdBody.Category = &ReadmeIoReference{URI: configuration.CategoryURI}
	}
	// Add parent if provided
	if strings.TrimSpace(parentSlug) != "" {
		createdBody.Parent = &ReadmeIoReference{URI: buildParentURI(parentSlug)}
	}
	createdBodyJSON, err := json.Marshal(createdBody)
	if err != nil {
		panic(fmt.Sprintf("Error marshalling JSON: %s", err))
	}
	resp := sendRequest("POST", "", bytes.NewBuffer(createdBodyJSON))
	if resp == nil {
		panic(fmt.Sprintf("Error creating page: %s (no response)", slug))
	}
	if resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		panic(fmt.Sprintf("Error creating page: %s (%s) - %s\n", slug, resp.Status, string(bodyBytes)))
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	updatedBody := ReadmeIoPageUpdate{
		Title: title,
		Content: &ReadmeIoPageContent{
			Body: content,
		},
	}
	bodyJSON, err := json.Marshal(updatedBody)
	if err != nil {
		panic(fmt.Sprintf("Error marshalling JSON: %s", err))
	}
	resp = sendRequest("PATCH", slug, bytes.NewBuffer(bodyJSON))
	if resp == nil {
		return false
	}
	ok := resp.StatusCode == http.StatusOK
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return ok
}

func buildParentURI(parentSlug string) string {
	// Build a URI that points to the parent page within the current branch and section (leading slash to match API URIs)
	return fmt.Sprintf("/branches/%s/%s/%s", configuration.Branch, configuration.Section, parentSlug)
}

func updatePage(slug string, title string, content string) bool {
	updatedBody := ReadmeIoPageUpdate{
		Title: title,
		Content: &ReadmeIoPageContent{
			Body: content,
		},
	}
	bodyJSON, err := json.Marshal(updatedBody)
	if err != nil {
		panic(fmt.Sprintf("Error marshalling JSON: %s", err))
	}
	resp := sendRequest("PATCH", slug, bytes.NewBuffer(bodyJSON))
	if resp == nil {
		return false
	}
	ok := resp.StatusCode == http.StatusOK
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return ok
}

func deletePage(slug string) bool {
	resp := sendRequest("DELETE", slug, nil)
	if resp == nil {
		return false
	}
	ok := resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusOK
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return ok
}

func sendRequest(method string, endpoint string, body io.Reader) *http.Response {
	var pathSuffix string
	if endpoint == "" {
		pathSuffix = ""
	} else {
		pathSuffix = "/" + endpoint
	}
	url := fmt.Sprintf("%s/branches/%s/%s%s", configuration.Endpoint, configuration.Branch, configuration.Section, pathSuffix)
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		fmt.Println("Error creating request:", err)
		return nil
	}
	req.Header.Set("authorization", fmt.Sprintf("Bearer %s", configuration.ApiKey))
	req.Header.Set("content-type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Error making request:", err)
		return nil
	}
	return resp
}

func getCategoryURI(title string) string {
	encodedTitle := url.PathEscape(strings.TrimSpace(title))
	fullURL := fmt.Sprintf("%s/branches/%s/categories/%s/%s", configuration.Endpoint, configuration.Branch, configuration.Section, encodedTitle)
	fmt.Printf("Reading category URI in '%s'\n", fullURL)

	req, err := http.NewRequest("GET", fullURL, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("authorization", fmt.Sprintf("Bearer %s", configuration.ApiKey))
	req.Header.Set("content-type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ""
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}

	// The API wraps the payload under a top-level "data" object
	var payload struct {
		Data struct {
			URI string `json:"uri"`
		} `json:"data"`
	}
	if err := json.Unmarshal(bodyBytes, &payload); err == nil && payload.Data.URI != "" {
		return payload.Data.URI
	}
	return ""
}

func generateSlug(str string) string {
	re := regexp.MustCompile(`[^a-zA-Z]+`)
	return strings.Trim(strings.ToLower(re.ReplaceAllString(str, "-")), "-_ ")
}

func cleanString(str string) string {
	return strings.Trim(str, " \n\t")
}

// escapeMarkdown converts characters like <, >, & and " to HTML entities
// to prevent them from being interpreted as HTML in markdown rendering.
// Do NOT use this for content inside fenced code blocks.
func escapeMarkdown(str string) string {
	return html.EscapeString(str)
}

// escapeOutsideInlineCode escapes only the parts of a string that are
// outside inline code spans delimited by backticks (`...`).
func escapeOutsideInlineCode(line string) string {
	if line == "" {
		return line
	}
	parts := strings.Split(line, "`")
	for idx := 0; idx < len(parts); idx++ {
		if idx%2 == 0 { // outside inline code
			parts[idx] = html.EscapeString(parts[idx])
		}
	}
	return strings.Join(parts, "`")
}

// escapePreservingFences escapes only outside of fenced code blocks (``` ... ```)
// and also preserves inline code spans using escapeOutsideInlineCode.
func escapePreservingFences(s string) string {
	if s == "" {
		return s
	}
	lines := strings.Split(s, "\n")
	inFence := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inFence = !inFence
			continue
		}
		if !inFence {
			lines[i] = escapeOutsideInlineCode(line)
		}
	}
	return strings.Join(lines, "\n")
}

func getQueryHeader(item Item) string {
	return fmt.Sprintf("**%s**\n", escapeMarkdown(cleanString(item.Name)))
}

func getHeader(level string, item Item) string {
	content := ""
	if level != "" {
		content += fmt.Sprintf("%s %s\n", level, escapeMarkdown(cleanString(item.Name)))
	}

	return content
}

func isEmptyRequest(req Request) bool {
	return req.Method == "" && req.URL == (RawURL{}) && req.Body == (Body{}) && len(req.Header) == 0 && req.Description == "" && req.Auth == (Auth{})
}

func isQuery(item Item) bool {
	return len(item.Item) == 0 && !isEmptyRequest(item.Request)
}
