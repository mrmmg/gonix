
# Nginx Manager TUI — Go Development Prompt

## Project Overview

I need a complete, production-oriented **Linux TUI (Terminal User Interface) application for managing Nginx**, implemented in **Go**.

I previously used **Nginx Proxy Manager (NPM)**. It provides a very good UI and many useful features, but it also limits direct control over Nginx configuration in some areas.

I therefore want to build my own **advanced, beautiful, interactive Linux TUI application for managing Nginx**.

The functionality should be inspired by Nginx Proxy Manager, but the implementation should give me **full control over the actual Nginx configuration files**, while keeping the project clean, modular, maintainable, Git-friendly, and easy to extend in the future.

The application **must be implemented in Go**, using a modular architecture with clearly separated packages/components.

This should be treated as a real application/project rather than a simple shell script.

---

# 1. Technology Requirements

## Programming Language

Use:

```text
Go
```

Do not implement the main application as Bash/Shell scripts.

Shell commands may still be invoked when appropriate for interacting with Linux/Nginx, for example:

```text
nginx -t
systemctl reload nginx
systemctl restart nginx
htpasswd
logrotate
```

However, the application logic, TUI, configuration management, certificate handling, validation, auditing, backups, and business logic should be implemented in Go.

Use idiomatic Go and follow standard Go project conventions.

---

# 2. TUI Framework

Build a polished interactive terminal UI.

You may use an appropriate Go TUI framework. Prefer a mature and well-maintained ecosystem such as:

* Bubble Tea
* Bubbles
* Lip Gloss

or another well-established Go TUI framework if you have a strong reason.

The UI should be:

* Interactive
* Keyboard-friendly
* Clean
* Modern
* Visually appealing
* Suitable for SSH sessions
* Consistent across all screens
* Easy to navigate

---

# 3. Main Menu

When the application starts, display a beautiful interactive main menu.

Initially include:

1. Add New Host
2. Manage Hosts
3. Certificates
4. Nginx Status
5. Access Lists
6. Settings
7. Exit

You may suggest additional useful top-level menu items if they make sense.

The application should support navigation using keyboard controls such as:

```text
↑ ↓
Enter
Esc
q
```

Provide clear shortcuts in the UI.

---

# 4. Add New Host

The "Add New Host" workflow should interactively ask the user for the required information.

## Domain name

Ask for the domain name and validate it.

Examples:

```text
example.com
www.example.com
api.example.com
```

Invalid domain names must be rejected with a clear error message.

## Host mode

Initially support:

* Reverse Proxy
* Direct / Static

Reverse Proxy is the primary use case.

After selecting Reverse Proxy, ask for relevant configuration such as:

* Upstream host/IP
* Upstream port
* HTTP/HTTPS upstream
* Proxy timeout
* Connection timeout
* Read timeout
* Send timeout
* WebSocket support
* HTTP version
* Forwarded headers
* Other useful reverse-proxy options

Design this in a way that allows additional options to be added later.

---

# 5. Nginx Configuration File Naming

Nginx configuration files should preferably be named after the domain.

For example:

```text
/etc/nginx/sites-available/example.com
/etc/nginx/sites-enabled/example.com
```

If the host is:

```text
api.example.com
```

the configuration file should be:

```text
/etc/nginx/sites-available/api.example.com
```

Do not use opaque generated IDs for normal host configuration files.

The application should use symbolic links between:

```text
sites-available
```

and:

```text
sites-enabled
```

---

# 6. Host Management

Selecting "Manage Hosts" should display existing hosts.

Hosts should be discovered from the Nginx configuration structure.

Each host should have its own management submenu.

At minimum provide:

* Enable / Disable
* Add New Path
* Add Custom Path
* SSL Configuration
* WebSocket Configuration
* Proxy Configuration
* Access List
* Access / Error Logs
* View Configuration
* Backup / Restore
* Delete Host

Additional useful options are welcome.

---

# 7. Enable / Disable Host

Before displaying the action, determine whether the host is enabled or disabled.

Use:

```text
/etc/nginx/sites-available/
/etc/nginx/sites-enabled/
```

If enabling a host, create the appropriate symbolic link.

If disabling a host, remove only the symbolic link.

Never delete the actual configuration file when disabling a host.

The UI should clearly show the current state:

```text
● ENABLED
○ DISABLED
```

---

# 8. Add New Path / Location

Allow the user to add another `location` block to an existing host.

The workflow should ask questions similar to "Add New Host".

For example:

```nginx
location /api/ {
    proxy_pass http://127.0.0.1:8000;
}
```

Support relevant reverse-proxy settings.

The resulting Nginx configuration must remain readable and valid.

---

# 9. Add Custom Path / Location

Provide:

```text
Add Custom Location
```

This should ask only for:

* Path
* Custom location configuration

The application should insert the custom location into the host configuration.

For example:

```nginx
location /custom/ {
    # User-defined configuration
}
```

The UI should inform the user that the configuration can also be manually edited later.

---

# 10. SSL Configuration

Provide an SSL configuration section for every host.

I manually obtain certificates using Certbot, usually with a DNS challenge.

I commonly use wildcard certificates.

I do **not** currently need automatic Let's Encrypt management.

Instead, use a centralized certificate directory.

For example:

```text
/etc/nginx/certs/
├── example.com/
│   ├── fullchain.pem
│   └── privkey.pem
├── example.org/
│   ├── fullchain.pem
│   └── privkey.pem
```

The generated Nginx configuration should reference these files directly:

```nginx
ssl_certificate /etc/nginx/certs/example.com/fullchain.pem;
ssl_certificate_key /etc/nginx/certs/example.com/privkey.pem;
```

Do not copy certificates into individual host directories.

This allows certificate renewal simply by replacing the centralized certificate files.

## Certificate Key Type

Support:

* RSA
* ECDSA

Design SSL handling so additional TLS options can be added later.

## Future Let's Encrypt Support

Do not implement automatic Let's Encrypt management now.

Add a clear TODO in the code and documentation for future Certbot / Let's Encrypt integration.

---

# 11. WebSocket Support

WebSocket support is an important feature.

Provide a simple option similar to Nginx Proxy Manager:

```text
Enable WebSocket Support: Yes / No
```

When enabled, automatically configure the required Nginx directives, including appropriate:

```text
proxy_http_version
Upgrade
Connection
```

headers.

The user should not need to manually write the WebSocket configuration.

---

# 12. Reverse Proxy Configuration

Reverse proxy configuration should be a first-class feature.

Support options such as:

* `proxy_pass`
* `proxy_http_version`
* `proxy_set_header`
* Host forwarding
* X-Forwarded-For
* X-Forwarded-Proto
* Client IP forwarding
* Connection upgrade
* Connect timeout
* Read timeout
* Send timeout
* Buffer settings
* Request body size
* Other useful proxy settings

The UI should expose commonly used settings without making the normal workflow unnecessarily complicated.

Advanced/custom configuration should remain possible.

---

# 13. Certificates Management

The "Certificates" menu should scan:

```text
/etc/nginx/certs/
```

Only show certificate directories containing both:

```text
fullchain.pem
privkey.pem
```

For each certificate display:

* Domain
* Status
* Expiration date
* Remaining time
* Certificate type
* Issuer
* Subject
* SANs
* Certificate path
* Private key path
* Whether it is expired

Expiration should be displayed in two forms.

### Human-readable

Example:

```text
Expires in approximately 20 days
```

### Exact

Use:

```text
Y-m-d H:i:s
```

Example:

```text
2026-10-05 16:18:59
```

Visually distinguish:

* Valid
* Expiring Soon
* Expired

Useful additional certificate functionality should be considered, including:

* View certificate details
* Verify certificate/key matching
* Verify certificate chain
* Detect invalid PEM files
* Show SHA-256 fingerprint
* Show issuer
* Show SANs
* Detect RSA vs ECDSA
* Test whether Nginx can load the certificate

Use Go's standard `crypto/x509` package where appropriate instead of unnecessarily invoking external commands.

---

# 14. Nginx Status

Create a dedicated Nginx status and management section.

Include:

## Service Status

* Running / stopped
* Enabled / disabled at boot

## Service Actions

* Start
* Stop
* Restart
* Reload
* Test Configuration

Always validate configuration before reload/restart operations where appropriate:

```bash
nginx -t
```

If validation fails, do not reload the configuration.

## Process Information

Display:

* Master process
* Worker processes
* Worker count
* Worker PIDs
* Total Nginx processes

## Basic Monitoring

Provide a lightweight overview containing:

* Nginx status
* CPU usage
* Memory usage
* Process count
* Uptime
* Listening ports
* Configuration test status

This does not need to become a full monitoring system.

---

# 15. Access Lists / HTTP Basic Authentication

Implement functionality similar to Nginx Proxy Manager's Access Lists.

The application should allow reusable HTTP Basic Authentication configurations.

For example:

```text
Access List: Admin Panel

Users:
    admin
    operator
```

Use appropriate password hashing and do not store plaintext passwords.

The application may use the system `htpasswd` utility if appropriate, but the business logic and management interface must remain in Go.

Allow:

1. Create access list
2. Edit access list
3. Delete access list
4. Add users
5. Remove users
6. Change passwords
7. View hosts using an access list
8. Apply an access list to a host
9. Remove an access list from a host

Generated Nginx configuration should use directives such as:

```nginx
auth_basic "Restricted";
auth_basic_user_file /path/to/htpasswd;
```

---

# 16. Access and Error Logs

For every host support:

* Enable/disable access log
* Enable/disable error log

Default:

```text
Access Log: Enabled
Error Log: Enabled
```

Show the actual log paths.

For example:

```text
/var/log/nginx/example.com.access.log
/var/log/nginx/example.com.error.log
```

Use standard Linux `logrotate`.

The application should either configure logrotate automatically during installation or provide a clear configuration/install mechanism.

---

# 17. Audit Log

Implement an audit logging system for changes made through the application.

Example:

```text
2026-09-15 10:32:11 | user=root | action=host_created | host=example.com
2026-09-15 10:35:20 | user=root | action=host_enabled | host=example.com
2026-09-15 10:41:02 | user=root | action=ssl_updated | host=example.com
```

Record at minimum:

* Timestamp
* Linux user
* Action
* Target
* Result
* Error information when applicable

Use log rotation.

Prefer standard Linux `logrotate`.

---

# 18. Configuration Safety

This is a critical requirement.

The application must be designed to avoid breaking Nginx.

Before applying configuration changes:

1. Create a backup when appropriate.
2. Generate/update the configuration.
3. Run:

```bash
nginx -t
```

4. If validation succeeds:

   * Apply/reload the configuration.

5. If validation fails:

   * Do not reload Nginx.
   * Display the Nginx error.
   * Restore the previous configuration when appropriate.
   * Record the failure in the audit log.

Implement a reliable backup/rollback mechanism.

For example:

```text
/etc/nginx-manager/backups/
```

Never blindly overwrite an existing configuration.

---

# 19. Nginx Configuration Architecture

The Go application should maintain an internal representation of Nginx hosts.

For example:

```text
Host
├── ServerNames
├── Listen
├── SSL
├── Locations[]
│   ├── Path
│   ├── ProxyPass
│   ├── WebSocket
│   └── CustomConfiguration
├── AccessLog
├── ErrorLog
└── AccessList
```

Separate:

1. Domain/business model
2. Configuration generation
3. Nginx filesystem operations
4. Validation
5. TUI
6. System operations

The TUI must not directly manipulate Nginx files.

Use a layered architecture.

---

# 20. Nginx Configuration Generation

Generated configuration should remain readable and human-editable.

For example:

```text
/etc/nginx/sites-available/example.com
```

should contain normal, understandable Nginx syntax.

Do not create an opaque proprietary configuration format that prevents administrators from editing the actual Nginx configuration.

The management application should be a layer on top of Nginx, not a replacement for Nginx.

---

# 21. Existing Configuration Compatibility

The application must gracefully handle Nginx configurations that were created manually.

Do not assume every configuration file was generated by this application.

The application should:

* Detect existing configurations
* Avoid destroying unknown configurations
* Warn before modifying manually managed files
* Allow custom configuration
* Handle existing `sites-available` and `sites-enabled`
* Detect broken symbolic links
* Detect potential conflicts

The administrator must always retain direct access to the underlying Nginx configuration.

If full parsing/editing of arbitrary existing Nginx configuration is not safely possible, do not attempt destructive automatic modification. Provide a safe custom/manual configuration workflow instead.

---

# 22. Project Architecture

The project must be implemented in Go using a modular architecture.

A suggested structure:

```text
nginx-manager/
├── cmd/
│   └── nginx-manager/
│       └── main.go
│
├── internal/
│   ├── tui/
│   │   ├── app.go
│   │   ├── styles.go
│   │   ├── menu.go
│   │   └── ...
│   │
│   ├── nginx/
│   │   ├── manager.go
│   │   ├── parser.go
│   │   ├── renderer.go
│   │   ├── validator.go
│   │   └── models.go
│   │
│   ├── hosts/
│   ├── certificates/
│   ├── accesslist/
│   ├── audit/
│   ├── backup/
│   ├── system/
│   ├── logs/
│   └── config/
│
├── templates/
│   ├── reverse-proxy.tmpl
│   ├── ssl.tmpl
│   ├── websocket.tmpl
│   └── ...
│
├── configs/
│   └── default.yaml
│
├── scripts/
│   └── install.sh
│
├── tests/
│
├── README.md
├── go.mod
├── go.sum
├── LICENSE
└── VERSION
```

You may improve this architecture if you have a better design.

The key requirement is clear separation of responsibilities.

---

# 23. Configuration File

The application should have a centralized configuration file.

For example:

```text
/etc/nginx-manager/nginx-manager.yaml
```

Possible settings:

```yaml
nginx:
  config_dir: /etc/nginx
  sites_available: /etc/nginx/sites-available
  sites_enabled: /etc/nginx/sites-enabled

certificates:
  directory: /etc/nginx/certs

logs:
  nginx_directory: /var/log/nginx
  audit_file: /var/log/nginx-manager/audit.log

backup:
  directory: /etc/nginx-manager/backups
```

Do not hard-code these paths throughout the application.

All configurable paths should be centralized.

---

# 24. Root / Permissions

Detect whether the application is running with sufficient permissions.

If root privileges are required, display a clear error.

For example:

```text
This operation requires root privileges.
Please run nginx-manager with sudo.
```

The expected usage should be:

```bash
sudo nginx-manager
```

---

# 25. Go Code Quality

Follow idiomatic Go practices.

Use:

* `go fmt`
* `go vet`
* Appropriate static analysis
* Unit tests
* Clear package boundaries
* Error wrapping
* Context where appropriate
* Structured logging where appropriate

Avoid unnecessary global state.

Avoid unnecessarily complicated abstractions.

Prefer simple, testable components.

The project should be compatible with modern stable Go versions.

Document the required Go version in `go.mod` and `README.md`.

---

# 26. Testing

Create tests for important functionality.

At minimum consider tests for:

* Domain validation
* Certificate parsing
* Certificate expiration calculation
* RSA/ECDSA detection
* Host configuration rendering
* Location rendering
* WebSocket configuration
* Enable/disable logic
* Symbolic link handling
* Backup/restore
* Configuration validation behavior
* Access list management

Avoid requiring a real production Nginx installation for unit tests whenever possible.

Use temporary directories and mocks/interfaces for filesystem and system operations.

---

# 27. Installation

Provide an installation mechanism.

For example:

```bash
sudo ./scripts/install.sh
```

The installer should:

* Detect the operating system
* Check required dependencies
* Check Nginx availability
* Check required external utilities
* Create required directories
* Install the binary
* Install configuration
* Configure logrotate
* Make the application available globally

After installation:

```bash
sudo nginx-manager
```

should launch the application.

---

# 28. Build System

The project must include clear build instructions.

The README must explain how to build the application from source.

At minimum document:

### Development build

```bash
go build -o nginx-manager ./cmd/nginx-manager
```

### Run locally

```bash
sudo ./nginx-manager
```

### Production build

Provide an optimized build command, for example:

```bash
go build -trimpath -ldflags="-s -w" -o nginx-manager ./cmd/nginx-manager
```

Explain what the flags do.

Also document:

* Required Go version
* `go mod download`
* `go mod tidy`
* `go test ./...`
* `go vet ./...`
* Optional cross-compilation
* How to install the resulting binary

If appropriate, provide a `Makefile` with commands such as:

```text
make build
make test
make lint
make install
make clean
```

The exact implementation is up to you.

---

# 29. Release / Distribution

Because this is a Go application, prefer distributing a compiled binary.

The README should explain both:

1. Running from source
2. Building and installing the binary

If practical, structure the project so future GitHub Releases can provide binaries for common Linux architectures such as:

```text
linux/amd64
linux/arm64
```

Do not make this unnecessarily complex in the initial version.

---

# 30. README

Create a comprehensive `README.md`.

It must include:

## Introduction

Explain the purpose of the project.

## Features

List supported features.

## Requirements

Include:

* Linux
* Go version for development
* Nginx
* Required system utilities

## Installation

Explain the installation process.

## Building from Source

Include complete commands:

```bash
git clone ...
cd nginx-manager

go mod download

go build -o nginx-manager ./cmd/nginx-manager
```

Explain how to run the application.

## Development Commands

Document:

```bash
go test ./...
go vet ./...
go fmt ./...
```

and any Makefile commands.

## Project Structure

Explain the Go package architecture.

## Nginx Configuration

Explain how hosts are stored.

## SSL Certificates

Explain:

```text
/etc/nginx/certs/example.com/fullchain.pem
/etc/nginx/certs/example.com/privkey.pem
```

## Access Lists

Explain their implementation.

## Logs

Explain:

* Access logs
* Error logs
* Audit logs
* Logrotate

## Backups

Explain backup and rollback behavior.

## Development

Explain how another developer can contribute.

## Future Features

Document planned features.

---

# 31. Git-Friendly Design

The project must be a clean Git repository.

Include:

```text
.gitignore
README.md
LICENSE
VERSION
go.mod
go.sum
```

Do not store runtime state in the repository.

Do not commit generated binaries.

Do not commit production configuration or secrets.

The project should be suitable for future CI/CD.

---

# 32. Future Extensibility

Design the application so future features can be added without rewriting the architecture.

Potential future features:

* Automatic Let's Encrypt / Certbot integration
* DNS challenge providers
* Cloudflare API integration
* IP allow/deny lists
* Rate limiting
* Security headers
* Compression
* Caching
* WAF integration
* ModSecurity integration
* Custom Nginx snippets
* Upstream groups
* Load balancing
* Health checks
* HTTP/2 / HTTP/3
* OCSP stapling
* HSTS
* HTTP → HTTPS redirects
* Maintenance mode
* Custom error pages
* Import existing Nginx configurations
* Configuration diff
* Git-based configuration history
* Rollback
* Multiple Nginx instances

Do not implement unnecessary future features now. Design the architecture so they can be added later.

---

# 33. Additional Useful Features

If you identify features that significantly improve reliability or usability, implement them when appropriate.

In particular, consider:

* Configuration diff before applying changes
* Automatic backup before modifications
* Rollback after failed validation
* Host search
* Certificate expiry warnings
* Broken symbolic-link detection
* Duplicate server-name detection
* Duplicate listen detection
* Nginx configuration validation
* Listening port overview
* Host configuration preview
* Safe reload
* Import existing hosts
* Configuration syntax preview
* Reverse-proxy upstream health checks
* Git-based configuration history

Do not add complexity simply for the sake of adding features.

Prioritize:

1. Reliability
2. Safety
3. Maintainability
4. Usability
5. Extensibility

---

# 34. UI / UX

The TUI should look and feel like a real application.

Example:

```text
╭──────────────────────────────────────────────────────────╮
│ NGINX MANAGER                                  v0.1.0   │
├──────────────────────────────────────────────────────────┤
│                                                          │
│  Hosts                                                   │
│  ─────────────────────────────────────────────────────   │
│                                                          │
│  ● example.com                         HTTPS   ENABLED   │
│  ● api.example.com                     HTTPS   ENABLED   │
│  ○ panel.example.com                   HTTP    DISABLED  │
│                                                          │
├──────────────────────────────────────────────────────────┤
│ ↑↓ Navigate    Enter Select    / Search    q Quit       │
╰──────────────────────────────────────────────────────────╯
```

Use consistent colors, symbols, spacing, tables, status indicators, and confirmation dialogs.

The UI should remain usable on a normal SSH terminal.

---

# 35. Important Implementation Principle

This application is a **management layer over Nginx**, not a replacement for Nginx.

The generated configuration must remain:

* Standard Nginx syntax
* Readable
* Human-editable
* Easy to debug manually

For example:

```bash
vim /etc/nginx/sites-available/example.com
```

should show normal, understandable Nginx configuration.

Never hide the actual configuration behind a proprietary database or opaque format.

---

# 36. Development Process

Before implementing the application:

1. Design the architecture.
2. Define the Go package structure.
3. Define the data models.
4. Define the TUI navigation model.
5. Define configuration management.
6. Define Nginx configuration rendering.
7. Define validation and rollback.
8. Define certificate management.
9. Define audit logging.
10. Define backup behavior.
11. Define testing strategy.

Then implement the project incrementally.

Each package should have a clear responsibility.

Avoid creating a single giant `main.go`.

---

# Final Goal

The final result should be a **production-quality, beautiful, interactive Linux TUI Nginx management application written in Go**.

It should provide:

* Host management
* Reverse proxy management
* Location management
* WebSocket configuration
* SSL management
* RSA/ECDSA support
* Certificate monitoring
* Access Lists / Basic Authentication
* Access/Error log management
* Log rotation
* Audit logging
* Nginx service management
* Worker/process monitoring
* Configuration validation
* Automatic backups
* Safe rollback
* Modular Go architecture
* Unit tests
* Comprehensive documentation
* Installation tooling
* Clear build instructions
* Git-friendly source code
* Future extensibility

The implementation should prioritize **safety, correctness, maintainability, usability, and extensibility** over simply maximizing the number of features.

The result should feel like a serious Linux administration tool, not a collection of shell commands wrapped in a TUI.