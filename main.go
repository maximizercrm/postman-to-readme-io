package main

import (
	"encoding/json"
	"fmt"
	"github.com/joho/godotenv"
	"io/ioutil"
	"os"
	"regexp"
	"strings"
)

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

func main() {
	// Load .env file
	err := godotenv.Load()
	if err != nil {
		fmt.Println("Error loading .env file:", err)
		return
	}

	sourceFile := os.Getenv("SOURCE_FILE")
	destinationFolder := os.Getenv("DESTINATION_FOLDER")

	// Read Postman collection
	file, err := ioutil.ReadFile(sourceFile)
	if err != nil {
		fmt.Println("Error reading Postman collection:", err)
		return
	}

	var postmanCollection struct {
		Item []Item `json:"item"`
	}

	err = json.Unmarshal(file, &postmanCollection)
	if err != nil {
		fmt.Println("Error unmarshalling Postman collection:", err)
		return
	}

	// Create docs directory if not exists
	err = os.MkdirAll(destinationFolder, os.ModePerm)
	if err != nil {
		fmt.Println("Error creating docs directory:", err)
		return
	}

	// Generate documentation pages
	for _, item := range postmanCollection.Item {
		createRootPage(item, destinationFolder)
	}
}

func createRootPage(item Item, destinationFolder string) {
	slug := fmt.Sprintf("octopus-%s", generateSlug(item.Name))
	header := "" //fmt.Sprintf("# %s\n\n", item.Name)
	description := ""
	if item.Description != "" {
		description = fmt.Sprintf("\n%s\n\n", cleanString(item.Description))
	}

	content := header + description + "\n"
	if len(item.Item) == 0 {
		content += processQuery(item, "#")
	}

	for _, subItem := range item.Item {
		if len(subItem.Item) == 0 {
			content += processQuery(subItem, "#")
		} else {
			subPageSlug := fmt.Sprintf("%s-%s", slug, generateSlug(subItem.Name))
			subFolderLink := fmt.Sprintf("[%s](/docs/%s)\n\n", subItem.Name, subPageSlug)
			content += subFolderLink
			createSubPage(subItem, subPageSlug, destinationFolder, "")
		}
	}

	filename := fmt.Sprintf("%s/%s.md", destinationFolder, slug)
	err := ioutil.WriteFile(filename, []byte(content), os.ModePerm)
	if err != nil {
		fmt.Println("Error writing file:", filename, err)
	}
}

func createSubPage(item Item, slug string, destinationFolder string, level string) {
	content := processSubItem(item, level)

	filename := fmt.Sprintf("%s/%s.md", destinationFolder, slug)
	err := ioutil.WriteFile(filename, []byte(content), os.ModePerm)
	if err != nil {
		fmt.Println("Error writing file:", filename, err)
	}
}

func processQuery(item Item, level string) string {
	content := fmt.Sprintf("%s %s\n", level, cleanString(item.Name))
	if item.Request.Description != "" {
		content += fmt.Sprintf("\n%s\n", cleanString(item.Request.Description))
	}
	url := strings.ReplaceAll(item.Request.URL.Raw, "{{BaseURL}}", "https://api.maximizer.com/octopus")
	authHeaders := "`Authorization: Bearer <token>`"
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
		if level != "" {
			content += fmt.Sprintf("%s %s\n", level, cleanString(item.Name))
		}
		if item.Description != "" {
			content += fmt.Sprintf("\n%s\n", cleanString(item.Description))
		}
		for _, subitem := range item.Item {
			content += processSubItem(subitem, fmt.Sprintf("%s#", level))
		}
	}
	return content
}

func generateSlug(str string) string {
	re := regexp.MustCompile(`[^a-zA-Z]+`)
	return strings.Trim(strings.ToLower(re.ReplaceAllString(str, "-")), "-_ ")
}

func cleanString(str string) string {
	return strings.Trim(str, " \n\t")
}
