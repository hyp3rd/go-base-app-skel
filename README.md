# Base Golang App Repository

<div align="center"> <svg width="200" height="200" viewBox="0 0 200 200" fill="none" xmlns="http://www.w3.org/2000/svg"> <rect width="200" height="200" fill="#1E1E1E"/> <path d="M40 40H160V160H40V40Z" fill="#4A4A4A" stroke="#00ADD8" stroke-width="8"/> <path d="M70 70L130 130" stroke="#00ADD8" stroke-width="12" stroke-linecap="square"/> <path d="M130 70L70 130" stroke="#00ADD8" stroke-width="12" stroke-linecap="square"/> <circle cx="100" cy="100" r="20" fill="#00ADD8"/> <rect x="85" y="20" width="30" height="20" fill="#00ADD8"/> <rect x="85" y="160" width="30" height="20" fill="#00ADD8"/> <rect x="20" y="85" width="20" height="30" fill="#00ADD8"/> <rect x="160" y="85" width="20" height="30" fill="#00ADD8"/> </svg> <h1>Base Golang App Repository</h1> <p>A skeleton repository for starting new Go applications with recommended structure and tooling.</p> </div>

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

3. Install VS Code Extensions Recommended (optional):

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

```txt
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
```

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

[MIT License](LICENSE)

## Contributing

1. Fork the repository
2. Create your feature branch
3. Commit your changes
4. Push to the branch
5. Create a Pull Request

Refer to [CONTRIBUTING.md](CONTRIBUTING.md) for more information.

## Author

I'm a surfer, and a software architect with 15 years of experience designing highly available distributed production systems and developing cloud-native apps in public and private clouds. Feel free to connect with me on LinkedIn.

[![LinkedIn](https://img.shields.io/badge/LinkedIn-0077B5?style=for-the-badge&logo=linkedin&logoColor=white)](https://www.linkedin.com/in/francesco-cosentino/)
