# TaskForge

TaskForge is a multi-tenant task and job management API built with Go, PostgreSQL, and Redis.

## Features

- Native JWT authentication.
- User registration and login.
- Multi-tenant architecture.
- Tenant membership and role-based access control (RBAC).
- Argon2id password hashing with bcrypt hash upgrade support.
- Short-lived access tokens.
- Refresh-token rotation and reuse detection.
- PostgreSQL persistence.
- Docker Compose development environment.

## Tech Stack

- **Language:** Go
- **Database:** PostgreSQL
- **Cache:** Redis
- **Authentication:** JWT
- **Password Hashing:** Argon2id
- **Containerization:** Docker and Docker Compose

## Architecture

```text
Client
  |
  v
HTTP API
  |
  +---- Public Authentication Routes
  |       |
  |       v
  |    Auth Service
  |       |
  |       +---- User Repository
  |       +---- Refresh Token Repository
  |       +---- JWT Authenticator
  |
  +---- Protected Job Routes
          |
          v
      JWT Authentication
          |
          v
      RBAC Middleware
          |
          v
      Job Service
          |
          v
      PostgreSQL
```

## Getting Started

### Prerequisites

- Go
- Docker
- Docker Compose
- PostgreSQL (if running outside Docker)

### Environment Configuration

Configure your `.env` file:

```env
DATABASE_URL=postgres://user:password@localhost:5432/taskforge?sslmode=disable

JWT_SECRET=replace-with-a-secure-secret-of-at-least-32-characters
JWT_ISSUER=taskforge
JWT_AUDIENCE=taskforge-api
```

Use a unique, securely generated JWT secret in each environment. Never commit production secrets.

### Run with Docker Compose

```bash
docker compose up --build
```

### Run Tests

```bash
go test ./...
```

## API Overview

### Health

| Method | Endpoint | Description |
|---|---|---|
| GET | `/healthz` | Health check |

### Authentication

| Method | Endpoint | Description |
|---|---|---|
| POST | `/api/v1/auth/register` | Register a user and create a tenant |
| POST | `/api/v1/auth/login` | Authenticate a user |
| POST | `/api/v1/auth/refresh` | Rotate refresh token |
| POST | `/api/v1/auth/logout` | Revoke refresh-token family |

### Jobs

| Method | Endpoint | Permission |
|---|---|---|
| POST | `/api/v1/jobs` | owner, admin, member |
| GET | `/api/v1/jobs/{id}` | owner, admin, member, viewer |

## Roles

| Role | Permissions |
|---|---|
| owner | Full tenant access |
| admin | Administrative access |
| member | Job creation and viewing |
| viewer | Read-only job access |

## Authentication Flow

1. A user registers or logs in.
2. The API verifies their credentials.
3. The API issues a short-lived JWT access token and an opaque refresh token.
4. The client sends the access token using the Bearer authorization scheme.
5. When the access token expires, the client exchanges its refresh token.
6. The API rotates the refresh token and issues a new token pair.
7. Reuse of a previously rotated refresh token triggers token-family revocation.

## Security

- Passwords are hashed using Argon2id.
- JWTs are signed using HS256.
- Refresh tokens are stored as SHA-256 hashes.
- Refresh tokens are single-use.
- Protected routes require authentication.
- RBAC enforces role-based endpoint permissions.
- Job repository operations enforce tenant isolation.

## Project Status

The authentication and initial job API endpoints are implemented. Additional job operations and tenant-management endpoints can be added as the project evolves.
```

## 2. API documentation

Create `docs/API.md`:

:::writing{variant="document" id="84519" title="TaskForge API.md"}
# TaskForge API Reference

Base URL:

```text
http://localhost:8080
```

## 1. Health Check

### GET `/healthz`

Checks whether the API is responding.

## 2. Authentication

All authentication endpoints are public. Protected job endpoints require a valid access token.

### Register

**POST** `/api/v1/auth/register`

Creates a user and their initial tenant.

Example request:

```json
{
  "email": "user@example.com",
  "password": "your-secure-password",
  "tenant_name": "Acme"
}
```

The API returns an access token and refresh token.

### Login

**POST** `/api/v1/auth/login`

Example request:

```json
{
  "email": "user@example.com",
  "password": "your-secure-password"
}
```

### Refresh

**POST** `/api/v1/auth/refresh`

Example request:

```json
{
  "refresh_token": "YOUR_REFRESH_TOKEN"
}
```

A successful refresh returns a new access token and refresh token.

The previous refresh token must no longer be used.

### Logout

**POST** `/api/v1/auth/logout`

Example request:

```json
{
  "refresh_token": "YOUR_REFRESH_TOKEN"
}
```

Revokes the associated refresh-token family.

## 3. Authorization

Protected endpoints require:

```http
Authorization: Bearer YOUR_ACCESS_TOKEN
```

### Role Permissions

| Role | Create Job | Read Job |
|---|---|---|
| owner | Yes | Yes |
| admin | Yes | Yes |
| member | Yes | Yes |
| viewer | No | Yes |

A request without valid authentication receives `401 Unauthorized`.

An authenticated user without the required role receives `403 Forbidden`.

## 4. Jobs

### Create Job

**POST** `/api/v1/jobs`

Required roles: `owner`, `admin`, `member`.

The request body follows the job DTO defined by the API implementation.

### Get Job

**GET** `/api/v1/jobs/{id}`

Required roles: `owner`, `admin`, `member`, `viewer`.

Returns a job belonging to the authenticated user's tenant.

Jobs belonging to other tenants must not be accessible.

## 5. Token Lifecycle

| Token | Lifetime | Storage |
|---|---|---|
| Access token | 15 minutes | Client |
| Refresh token | 7 days | Client; SHA-256 hash in database |

Refresh tokens are rotated on successful use.

If an already-rotated token is reused, the associated token family is revoked.

## 6. HTTP Status Codes

| Status | Meaning |
|---|---|
| 200 | Request successful |
| 201 | Resource created |
| 400 | Invalid request |
| 401 | Authentication required or invalid credentials |
| 403 | Insufficient permissions |
| 404 | Resource not found |
| 409 | Resource conflict |
| 500 | Internal server error |

Exact response codes depend on the individual handler implementation.
```

## 3. Update your project structure

Your documentation directory should look like this:

```text
taskforge/
├── cmd/
│   └── api/
├── internal/
│   ├── api/
│   ├── auth/
│   ├── database/
│   ├── jobs/
│   └── repository/
├── migrations/
├── docs/
│   └── API.md
├── README.md
├── docker-compose.yml
├── Dockerfile
├── go.mod
└── go.sum
```

**Important:** These drafts document the routes and behavior established in our conversation. Before publishing, verify the exact request DTOs, response schemas, error codes, and environment variable names against your current source code.

Next, we can add OpenAPI 3.1 documentation so TaskForge has an interactive Swagger API reference.
