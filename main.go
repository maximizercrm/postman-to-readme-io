package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/joho/godotenv"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
)

type Configuration struct {
	Endpoint     string
	ApiKey       string
	Prefix       string
	CategorySlug string
	PageSlug     string
	Version      string
	BaseURL      string
	Pages        PagesConfiguration
}

type PagesConfiguration struct {
	MarkdownFolder string
	PagesFile      string
	OldPages       []string
	NewPages       []string
}

type ReadmeIoPage struct {
	Type          string `json:"type,omitempty"`
	Title         string `json:"title,omitempty"`
	Body          string `json:"body,omitempty"`
	Hidden        bool   `json:"hidden"`
	CategorySlug  string `json:"categorySlug,omitempty"`
	ParentDocSlug string `json:"parentDocSlug,omitempty"`
}

type ReadmeIoPageUpdate struct {
	Type          string `json:"type,omitempty"`
	Title         string `json:"title,omitempty"`
	Body          string `json:"body,omitempty"`
	CategorySlug  string `json:"categorySlug,omitempty"`
	ParentDocSlug string `json:"parentDocSlug,omitempty"`
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
		panic(fmt.Sprintf("Error: COLLECTION_SOURCE_FILE is required"))
	}
	file, err := os.ReadFile(sourceFile)
	if err != nil {
		panic(fmt.Sprintf("Error reading Postman collection:", err))
	}

	configuration.Pages.MarkdownFolder = os.Getenv("MARKDOWN_FOLDER")
	if configuration.Pages.MarkdownFolder == "" {
		panic(fmt.Sprintf("Error: MARKDOWN_FOLDER is required"))
	}
	// Create docs directory if not exists
	err = os.MkdirAll(configuration.Pages.MarkdownFolder, os.ModePerm)
	if err != nil {
		panic(fmt.Sprintf("Error creating docs directory:", err))
	}

	configuration.PageSlug = os.Getenv("README_API_PAGES_SLUG")
	if configuration.PageSlug == "" {
		panic(fmt.Sprintf("Error: README_API_PAGES_SLUG is required"))
	}
	configuration.BaseURL = os.Getenv("COLLECTION_BASE_URL")

	configuration.Prefix = os.Getenv("README_API_PREFIX")
	if configuration.Prefix == "" {
		panic(fmt.Sprintf("Error: README_API_PREFIX is required"))
	}

	var postmanCollection struct {
		Item []Item `json:"item"`
	}

	err = json.Unmarshal(file, &postmanCollection)
	if err != nil {
		panic(fmt.Sprintf("Error unmarshalling Postman collection:", err))
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
	configuration.CategorySlug = os.Getenv("README_API_CATEGORY_SLUG")
	if configuration.CategorySlug == "" {
		fmt.Println("Markdown generated. Publish process stopped: README_API_CATEGORY_SLUG is empty")
		return
	}
	configuration.Version = os.Getenv("README_API_VERSION")
	if configuration.Version == "" {
		fmt.Println("Markdown generated. Publish process stopped: README_API_VERSION is empty")
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
		panic(fmt.Sprintf("Error deleting pages"))
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
			panic(fmt.Sprintf("Error writing to file:", err))
			return true
		}
	}

	// Flush the writer to ensure all data is written to the file
	err = writer.Flush()
	if err != nil {
		panic(fmt.Sprintf("Error flushing writer:", err))
	}
	return false
}

func loadPreviouslyCreatedPages(file string) []string {
	createdPagesFile, err := os.Open(file)
	if err != nil {
		panic(fmt.Sprintf("Error opening file:", err))
		return nil
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
		description = fmt.Sprintf("\n%s\n\n", cleanString(item.Description))
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
			subFolderLink := fmt.Sprintf("[%s](/%s/%s)\n", subItem.Name, configuration.PageSlug, subPageSlug)
			listContent += fmt.Sprintf("- %s", subFolderLink)
			createSubPage(slug, subItem, subPageSlug, destinationFolder, "")
			hasList = true
		}
	}
	if hasList {
		if hasContent {
			content += "\n***\n"
		}
		content += fmt.Sprintf("\n# Subsections\n")
		content += listContent
	}

	// Write content to file
	filename := fmt.Sprintf("%s/%s.md", destinationFolder, slug)
	err := ioutil.WriteFile(filename, []byte(content), os.ModePerm)
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
	err := ioutil.WriteFile(filename, []byte(content), os.ModePerm)
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
		content += fmt.Sprintf("\n%s\n", cleanString(item.Request.Description))
	}
	url := getRequestURL(item.Request)
	headers := getRequestHeaders(item.Request)
	requestExample := fmt.Sprintf("\n```json js%s%s\n%s\n```\n\n", url, headers, cleanString(item.Request.Body.Raw))
	responseExamples := ""
	if len(item.Response) > 0 {
		for _, response := range item.Response {
			responseExamples += fmt.Sprintf("\n**Example: %s**\n", cleanString(response.Name))
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
			content += fmt.Sprintf("\n%s\n", cleanString(item.Description))
		}
		for _, subitem := range item.Item {
			content += processSubItem(subitem, fmt.Sprintf("%s#", level))
		}
	} else {
		content += fmt.Sprintf("\n%s", item.Description)
	}
	return content
}

func upsertPage(parentSlug string, slug string, title string, content string) {
	configuration.Pages.NewPages = append(configuration.Pages.NewPages, slug)
	pageExists := checkPageExists(slug)
	if pageExists {
		fmt.Printf("Update the page %s\n", slug)
		updatePage(parentSlug, slug, title, content)
	} else {
		fmt.Printf("Create the page %s\n", slug)
		createPage(parentSlug, slug, title, content)
	}
}

func checkPageExists(slug string) bool {
	resp := sendRequest("GET", slug, nil)
	return resp.StatusCode == http.StatusOK
}

func createPage(parentSlug string, slug string, title string, content string) bool {
	createdBody := ReadmeIoPage{
		Type:          "basic",
		Title:         slug,
		Body:          content,
		Hidden:        true,
		CategorySlug:  configuration.CategorySlug,
		ParentDocSlug: parentSlug,
	}
	createdBodyJSON, err := json.Marshal(createdBody)
	if err != nil {
		panic(fmt.Sprintf("Error marshalling JSON: %s", err))
	}
	resp := sendRequest("POST", "", bytes.NewBuffer(createdBodyJSON))
	if resp.StatusCode != http.StatusCreated {
		panic(fmt.Sprintf("Error creating page: %s (%s)\n", slug, resp.Status))
	}

	updatedBody := ReadmeIoPage{
		Title:  title,
		Body:   content,
		Hidden: false,
	}
	bodyJSON, err := json.Marshal(updatedBody)
	if err != nil {
		panic(fmt.Sprintf("Error marshalling JSON: %s", err))
	}
	resp = sendRequest("PUT", slug, bytes.NewBuffer(bodyJSON))
	return resp.StatusCode == http.StatusOK
}

func updatePage(parentSlug string, slug string, title string, content string) bool {
	updatedBody := ReadmeIoPageUpdate{
		Title:         title,
		Body:          content,
		CategorySlug:  configuration.CategorySlug,
		ParentDocSlug: parentSlug,
	}
	bodyJSON, err := json.Marshal(updatedBody)
	if err != nil {
		panic(fmt.Sprintf("Error marshalling JSON: %s", err))
	}
	resp := sendRequest("PUT", slug, bytes.NewBuffer(bodyJSON))
	return resp.StatusCode == http.StatusOK
}

func deletePage(slug string) bool {
	resp := sendRequest("DELETE", slug, nil)
	return resp.StatusCode == http.StatusNoContent
}

func sendRequest(method string, endpoint string, body io.Reader) *http.Response {
	if endpoint == "" {
		endpoint = ""
	} else {
		endpoint = "/" + endpoint
	}
	req, err := http.NewRequest(method, fmt.Sprintf("%s/docs/%s", configuration.Endpoint, endpoint), body)
	if err != nil {
		fmt.Println("Error creating request:", err)
		return nil
	}
	req.Header.Set("authorization", fmt.Sprintf("Basic %s", configuration.ApiKey))
	req.Header.Set("content-type", "application/json")
	req.Header.Set("x-readme-version", configuration.Version)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Error making request:", err)
		return nil
	}
	defer resp.Body.Close()

	return resp
}

func generateSlug(str string) string {
	re := regexp.MustCompile(`[^a-zA-Z]+`)
	return strings.Trim(strings.ToLower(re.ReplaceAllString(str, "-")), "-_ ")
}

func cleanString(str string) string {
	return strings.Trim(str, " \n\t")
}

func getQueryHeader(item Item) string {
	return fmt.Sprintf("**%s**\n", cleanString(item.Name))
}

func getHeader(level string, item Item) string {
	content := ""
	if level != "" {
		content += fmt.Sprintf("%s %s\n", level, cleanString(item.Name))
	}

	return content
}

func isEmptyRequest(req Request) bool {
	return req.Method == "" && req.URL == (RawURL{}) && req.Body == (Body{}) && len(req.Header) == 0 && req.Description == "" && req.Auth == (Auth{})
}

func isQuery(item Item) bool {
	return len(item.Item) == 0 && !isEmptyRequest(item.Request)
}
