# VibeExperience Application Frontend

This is the frontend application for VibeExperience, built with React, Vite, TypeScript, and Tailwind CSS.

## Getting Started

```bash
npm run dev
```

Open [http://localhost:5173](http://localhost:5173) to view it in your browser.

## Available Scripts

### Development

- `npm run dev` - Start development server
- `npm run build` - Build for production
- `npm run preview` - Preview production build

### Code Quality

- `npm run lint` - Run ESLint
- `npm run lint:fix` - Fix ESLint issues
- `npm run format` - Format code with Prettier
- `npm run format:check` - Check code formatting
- `npm run type-check` - TypeScript type checking

### Security & Complexity

- `npm run security:scan` - Run security linting
- `npm run complexity:check` - Check code complexity (max warnings: 0)
- `npm run audit:deps` - Audit dependencies for vulnerabilities
- `npm run audit:deps:fix` - Fix dependency vulnerabilities

### Testing

- `npm run test` - Run tests
- `npm run test:watch` - Run tests in watch mode
- `npm run test:coverage` - Run tests with coverage
- `npm run test:e2e` - Run end-to-end tests
- `npm run test:e2e:ui` - Run E2E tests in UI mode
- `npm run test:e2e:debug` - Debug E2E tests

## Tech Stack

- **React 19** - UI library
- **Vite** - Build tool and dev server
- **TypeScript** - Type safety
- **Tailwind CSS v4** - Utility-first CSS framework
- **ESLint** - Code linting
- **Prettier** - Code formatting
- **Jest** - Testing framework

## Code Quality Standards

This project enforces strict code quality standards through automated checks:

### Security

- **Security Linting**: All code is scanned for common security vulnerabilities using `eslint-plugin-security`
- **Dependency Auditing**: Dependencies are audited for known vulnerabilities (moderate level and above)
- Run `npm run security:scan` to check for security issues
- Run `npm run audit:deps` to audit dependencies

### Complexity Limits

The following complexity thresholds are enforced:

- **Cyclomatic Complexity**: Maximum 15 per function
- **Cognitive Complexity**: Maximum 15 per function
- **Max Function Lines**: 150 lines per function (excluding blank lines and comments)
- **Max File Lines**: 500 lines per file (excluding blank lines and comments)

Run `npm run complexity:check` to verify complexity compliance.

### Pre-Commit Checklist

Before committing code, ensure all checks pass:

```bash
npm run lint          # Check code style
npm run type-check    # Verify TypeScript types
npm run test          # Run unit tests
npm run security:scan # Check security issues
npm run complexity:check # Verify complexity limits
npm run build         # Ensure production build works
```

### Fixing Common Issues

#### Security Warnings

- Review the specific security warning and assess if it's a false positive
- If legitimate, refactor the code to eliminate the vulnerability
- If false positive, add `// eslint-disable-next-line security/rule-name` with a justification comment

#### Complexity Issues

- Break down large functions into smaller, focused functions
- Extract repetitive logic into reusable utilities
- Simplify nested conditionals using early returns or guard clauses
- Consider using state machines or strategy patterns for complex logic

#### Dependency Vulnerabilities

- Run `npm run audit:deps:fix` to automatically fix vulnerabilities
- For unfixable vulnerabilities, assess the risk and update dependencies manually
- Check for breaking changes before upgrading major versions

### CI/CD Pipeline

All pull requests must pass the following automated checks:

1. ESLint code style checks
2. Security scanning
3. Dependency vulnerability audit
4. Code complexity verification
5. TypeScript type checking
6. Unit tests with coverage reporting
7. Production build verification

The CI pipeline will fail if any check does not pass, ensuring only high-quality code is merged.
