# Postman collection to readme.io documentation

## How it works

1. Based on the Postman collection, the script updates the developer portal and stores the list of files in the `readme-io-pages-new` file.
2. The scripts compares the `readme-io-pages-new` with `readme-io-pages` to identify the old pages.
3. The script will delete the old pages from the developer portal.
4. The script replaces the `readme-io-pages` with `readme-io-pages-new`.