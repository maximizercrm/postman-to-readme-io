# Generate documentation at readme.io based on your Postman collection

This script is used to convert the Postman collection to the readme.io documentation and create/update the documentation in the your readme.io developer portal.

## How it works

1. Based on the Postman collection, the script will generate the markdown files for the documentation. So, you can see result in the `MARKDOWN_FOLDER` folder without pushing it to the readme.io.
2. If the `README_API_*` variables are set in the `.env` file, the script will create/update the documentation in the readme.io developer portal.
   1. The script checks if a page with the specific slug exists in the developer portal.
   2. If the page exists, the script will update the page with the new content.
   3. If the page does not exist, the script will create a new page. At first, it creates a hidden page with the title of the slug and then updates the page with the new content and makes it visible. Readne.io does not allow to create a page with the specific slug directly.
3. The script will compare the `README_API_CREATED_PAGES_FILE` and the list of the slugs of the created/updated pages in the readme.io at the previous step. All slugs listed in the `README_API_CREATED_PAGES_FILE` but not in the list of the created/updated pages will be deleted from the readme.io developer portal.
4. The script will update the `README_API_CREATED_PAGES_FILE` with the list of the created/updated pages in the readme.io developer portal.

## How to use

1. Download the binary from the [releases](/releases)
2. Create a `.env` file based on the `.env.example` file in the same folder with the binary.
3. Run the binary 
