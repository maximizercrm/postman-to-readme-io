package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/joho/godotenv"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
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
	Name        string  `json:"name"`
	Description string  `json:"description,omitempty"`
	Item        []Item  `json:"item,omitempty"`
	Request     Request `json:"request,omitempty"`
}

type Request struct {
	Method      string   `json:"method"`
	URL         URL      `json:"url"`
	Body        Body     `json:"body"`
	Header      []Header `json:"header"`
	Description string   `json:"description,omitempty"`
	Auth        Auth     `json:"auth,omitempty"`
}

type Auth struct {
	Type string `json:"type,omitempty"`
}

type URL struct {
	Raw string `json:"raw"`
}

type Body struct {
	Raw string `json:"raw"`
}

type Header struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

var configuration Configuration
var pages Pages

func main() {
	// Load .env file
	err := godotenv.Load()
	if err != nil {
		panic(fmt.Sprintf("Error loading .env file:", err))
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
	configuration.Prefix = os.Getenv("README_API_PREFIX")
	if configuration.Prefix == "" {
		fmt.Println("Markdown generated. Publish process stopped: README_API_PREFIX is empty")
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

	// Find items in array1 that do not exist in OldPages array
	var diff []string
	for _, item := range configuration.Pages.OldPages {
		if !itemsMap[item] {
			diff = append(diff, item)
		}
	}

	// Delete pages that are not in the new list
	for _, item := range diff {
		if deletePage(item) {
			fmt.Printf("Delete the page %s\n", item)
		}
	}

	updatePagesList(configuration.Pages.PagesFile, configuration.Pages.NewPages)
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
	slug := fmt.Sprintf("octopus-postman-%s", generateSlug(item.Name))
	header := ""
	description := ""
	if item.Description != "" {
		description = fmt.Sprintf("\n%s\n\n", cleanString(item.Description))
	}

	content := header + description + "\n"
	if len(item.Item) == 0 {
		content += processQuery(item, "")
	}

	for _, subItem := range item.Item {
		if len(subItem.Item) == 0 {
			content += processQuery(subItem, "#")
		} else {
			subPageSlug := fmt.Sprintf("%s-%s", slug, generateSlug(subItem.Name))
			subFolderLink := fmt.Sprintf("[%s](/%s/%s)\n", subItem.Name, configuration.PageSlug, subPageSlug)
			content += fmt.Sprintf("- %s", subFolderLink)
			createSubPage(slug, subItem, subPageSlug, destinationFolder, "")
		}
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

func processQuery(item Item, level string) string {
	content := getQueryHeader(item, level)
	if item.Request.Description != "" {
		content += fmt.Sprintf("\n%s\n", cleanString(item.Request.Description))
	}
	url := strings.ReplaceAll(item.Request.URL.Raw, "{{BaseURL}}", configuration.BaseURL)
	authHeaders := "Authorization: Bearer <token>"
	if item.Request.Auth.Type == "noauth" {
		authHeaders = ""
	}
	requestTo := fmt.Sprintf("\n// %s %s", item.Request.Method, cleanString(url))
	headers := ""
	if authHeaders != "" {
		headers = fmt.Sprintf("\n// %s", authHeaders)
	}
	requestExample := fmt.Sprintf("\n```json js%s%s\n%s\n```\n\n", requestTo, headers, cleanString(item.Request.Body.Raw))
	return content + requestExample
}

func processSubItem(item Item, level string) string {
	content := ""
	if item.Request.Method != "" {
		content += processQuery(item, level)
	} else if len(item.Item) > 0 {
		content += getHeader(level, item)
		if item.Description != "" {
			content += fmt.Sprintf("\n%s\n", cleanString(item.Description))
		}
		for _, subitem := range item.Item {
			content += processSubItem(subitem, fmt.Sprintf("%s#", level))
		}
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

func getQueryHeader(item Item, level string) string {
	if level == "#" {
		return getHeader(level, item)
	}

	return fmt.Sprintf("**%s**\n", cleanString(item.Name))
}

func getHeader(level string, item Item) string {
	content := ""
	if level != "" {
		content += fmt.Sprintf("%s %s\n", level, cleanString(item.Name))
	}

	return content
}
