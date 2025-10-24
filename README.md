# AI News Processor

A high-performance news processing service that fetches, processes, and serves news content with AI-powered enhancements.

## Features

- **Feed Processing**: Fetches and parses news feeds from multiple sources
- **AI Enhancement**: Uses Gemini AI to enhance and process news content
- **Caching**: Implements Redis-based caching for improved performance
- **REST API**: Provides a clean Fiber-based HTTP API
- **Docker Support**: Easy containerization with Docker and Docker Compose

## Prerequisites

- Go 1.25 or higher
- Redis 7.0 or higher
- Docker (optional, for containerized deployment)
- Gemini API key

## Getting Started

1. Clone the repository:
   ```bash
   git clone https://github.com/bilgisen/goen.git
   cd goen
   ```

2. Copy the example environment file and update with your configuration:
   ```bash
   cp .env.example .env
   # Edit .env with your configuration
   ```

3. Install dependencies:
   ```bash
   go mod download
   ```

4. Run the application:
   ```bash
   go run cmd/main.go
   ```

## Configuration

Edit the `.env` file to configure the application. See `.env.example` for all available options.

## API Documentation

Once the server is running, you can access the following endpoints:

- `GET /health` - Health check endpoint
- `GET /api/v1/news` - Get processed news
- `POST /api/v1/process` - Process new feeds

## Deployment

### Docker

Build and run using Docker Compose:

```bash
docker-compose up --build
```

### Kubernetes

See the `deploy/` directory for Kubernetes deployment manifests.

## Development

### Running Tests

```bash
go test ./...
```

### Building

```bash
go build -o bin/ai-news-processor cmd/main.go
```

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
