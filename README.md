# Update documentation at readme.io based on your Postman collection

This tool converts the Postman collection to markdown files and creates/updates the documentation at your readme.io developer portal.

## How it works

1. Based on the Postman collection, the tool will generate the markdown files for the documentation. So, you can see result in the `MARKDOWN_FOLDER` folder without pushing it to the readme.io.
2. If the `README_API_*` variables are set in the `.env` file, the tool will create and update the documentation in the readme.io developer portal.
   1. The tool checks if a page with the specific slug exists in the developer portal.
   2. If the page exists, the tool will update the page with the new content.
   3. If the page does not exist, the tool will create a new one. At first, it creates a hidden page with the slug's title, then updates the page with the new content and makes it visible. Readne.io does not allow creating a page with the specific slug directly.
3. The tool will compare the `README_API_CREATED_PAGES_FILE` and the list of the slugs of the created/updated pages in the readme.io at the previous step. All slugs listed in the `README_API_CREATED_PAGES_FILE` but not in the list of the created/updated pages will be deleted from the readme.io developer portal.
4. The tool will update the `README_API_CREATED_PAGES_FILE` with the list of the created/updated pages in the readme.io developer portal.

NOTE: The tool does not update the visibility of the existing pages in the readme.io developer portal.

## How to use

1. Download the binary from the [releases](https://github.com/maximizercrm/postman-to-readme-io/releases)
2. Create a `.env` file based on the [.env.example](/.env.example) file in the same folder with the binary.
3. Run the binary 
