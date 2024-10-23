# Base Golang App Repository

A skeleton repository for starting new Go applications with recommended structure and tooling.

## Getting Started

1. Clone this repository:

   git clone <https://github.com/hyp3rd/go-base-app-skel.git> your-new-project

2. Remove the existing git folder and initialize a new repository:

   ```bash
   cd your-new-project
   rm -rf .git
   git init
   go mod init github.com/your-org/your-new-project
   ```

3. Install VS Code Extensions Recommended by this repository:

   ```json
   {
      "recommendations": [
         "github.vscode-github-actions",
         "golang.go",
         "ms-vscode.makefile-tools",
         "esbenp.prettier-vscode",
         "pbkit.vscode-pbkit",
         "trunk.io",
         "streetsidesoftware.code-spell-checker",
         "ms-azuretools.vscode-docker",
         "eamodio.gitlens"
      ]
   }
   ```

4. Initialize pre-commit and its hooks:

```bash
   pip install pre-commit
   pre-commit install
   pre-commit install-hooks
```

## Project Structure

├── cmd/ # Main applications
│   └── app/ # Your application
│       └── main.go # Application entry point
├── internal/ # Private code
│   ├── pkg/ # Internal packages
│   └── app/ # Application specific code
├── pkg/ # Public libraries
├── api/ # API contracts (proto files, OpenAPI specs)
├── configs/ # Configuration files
├── scripts/ # Scripts for development
├── test/ # Additional test files
└── docs/ # Documentation

## Development Setup

1. Install [Golang](https://go.dev/dl).
2. Install [Docker](https://docs.docker.com/get-docker/).
3. Install [GitVersion](https://github.com/GitTools/GitVersion).
4. Install [git](https://git-scm.com/downloads).
5. Install [Make](https://www.gnu.org/software/make/), follow the procedure for your OS.
6. Set up the toolchain:

   ```bash
   make prepare-toolchain
   ```

## Best Practices

- Follow the [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments)
- Run `golangci-lint` before committing code
- Ensure the pre-commit hooks pass
- Write tests for new functionality
- Keep packages small and focused
- Use meaningful package names
- Document exported functions and types

## Available Make Commands

- `make test`: Run tests.
- `make update-deps`: Update all dependencies in the project.
- `make prepare-toolchain`: Install all tools required to build the project.
- `make lint`: Run the staticcheck and golangci-lint static analysis tools on all packages in the project.
- `make run`: Build and run the application in Docker.

## License

MIT License

## Contributing

1. Fork the repository
2. Create your feature branch
3. Commit your changes
4. Push to the branch
5. Create a Pull Request
