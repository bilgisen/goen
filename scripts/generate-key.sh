#!/bin/bash
# Generate a secure random API key

# Check if openssl is installed
if ! command -v openssl &> /dev/null; then
    echo "Error: openssl is required but not installed." >&2
    exit 1
fi

# Generate a 64-character random string
API_KEY=$(openssl rand -base64 48 | tr -dc 'a-zA-Z0-9' | head -c 64)

echo "Generated secure API key:"
echo "$API_KEY"

# Instructions for .env file
echo -e "\nAdd this to your .env file:"
echo "ADMIN_API_KEY=$API_KEY"
