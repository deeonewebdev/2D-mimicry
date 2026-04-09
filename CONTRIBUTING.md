# Contributing to 2D Mimicry

Thank you for your interest! We welcome bug reports, feature suggestions, and pull requests.

## Code of Conduct
Please be respectful and constructive.

## How to contribute

1. **Open an issue** first to discuss significant changes.
2. **Fork the repository** and create a new branch from `main`.
3. **Write clear, tested code** (where applicable). The tools rely only on the standard library, so no external dependencies should be introduced.
4. **Run the existing examples** to ensure you didn’t break anything.
5. **Submit a pull request** with a clear description of what you changed and why.

## Development setup

- Use Go 1.21+.
- Clone your fork and run `go build` on each `.go` file.
- Test with the provided example data (create your own small wordlist).

## Style guide

- Follow `gofmt` and `go vet` suggestions.
- Keep functions focused and well‑named.
- Add comments for any non‑obvious logic.

## Reporting issues

Include:
- The exact command you ran.
- The full error output (if any).
- A small sample of input data that reproduces the problem.

Thank you for helping make 2D Mimicry better!
