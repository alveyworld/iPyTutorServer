# Proposal: Scalable DigitalOcean Deployment for OpenRouter Frontend

## Objective

Deploy a simple, scalable web application on DigitalOcean that serves a frontend for OpenRouter while keeping the OpenRouter API key secure on the server side.

The solution should avoid Marketplace images, App Platform, Node.js, and Python, while remaining easy to operate and horizontally scalable.

## Recommended Architecture

Use a Go web server running on plain Ubuntu Droplets inside a DigitalOcean Droplet Autoscale Pool, placed behind a DigitalOcean Load Balancer.

The Go web server should serve both:

1. The static frontend assets, including HTML, CSS, and browser-side JavaScript.
2. The backend API endpoint that proxies requests to OpenRouter.

text User Browser     ↓ HTTPS DigitalOcean Load Balancer     ↓ HTTP over private network Autoscaled Ubuntu Droplets     ↓ Go web server     ├── Static frontend: HTML, CSS, JavaScript     └── Backend API: OpenRouter proxy             ↓         OpenRouter API 

This keeps the deployment simple because each Droplet runs the same self-contained Go application. There is no need for a separate static file server unless the application later grows to require a CDN, independent frontend deployment, or heavy global static asset caching.

## Technology Stack

The application should be implemented using Go’s built-in net/http package. This avoids unnecessary framework complexity while providing a fast, reliable, compiled web server.

The Go service will serve the static frontend and expose a small backend API endpoint, such as /api/chat, which forwards requests to OpenRouter.

Recommended routes:

text GET  /              Static frontend entry point GET  /style.css     Static CSS file GET  /app.js        Browser-side JavaScript POST /api/chat      Server-side OpenRouter proxy GET  /healthz       Load balancer health check 

The browser will load the static HTML, CSS, and JavaScript from the Go server. The JavaScript running in the browser will then send requests to the Go application server, for example by calling /api/chat.

The JavaScript should not call OpenRouter directly. Instead, it should call the Go backend API. The Go server will read the OpenRouter API key from an environment variable and attach it to the server-side request to OpenRouter.

This ensures that the OpenRouter API key is never exposed to browser-side JavaScript.

## Static Frontend Serving

For this application, the static portion of the webpage should be served directly by the Go server.

This approach is recommended because:

- The frontend is simple.
- The backend is already required to protect the OpenRouter API key.
- Each Droplet can run a single Go binary.
- The deployment remains easy to understand and operate.
- The DigitalOcean Load Balancer can route both frontend and backend traffic to the same service.
- There is no need to operate a separate static web server.

A typical request flow would be:

text Browser     GET /         → Go serves index.html  Browser     GET /style.css         → Go serves CSS  Browser     GET /app.js         → Go serves JavaScript  Browser     POST /api/chat         → Go sends request to OpenRouter using server-side API key 

A separate static file server or CDN could be added later if needed, but it is not necessary for the initial version of this service.

## Infrastructure

The deployment should use:

text DigitalOcean Load Balancer Droplet Autoscale Pool Ubuntu 24.04 base image Go application binary systemd service Private networking 

The Load Balancer will terminate HTTPS and distribute traffic across healthy Droplets. Each Droplet will run the same Go binary as a systemd service listening on an internal port such as 8080.

The Load Balancer should forward requests to the Go application over the private network.

## Scaling Strategy

Horizontal scaling will be handled by the Droplet Autoscale Pool. The pool should start with a minimum of two Droplets for availability and scale out based on CPU or memory utilization.

The Load Balancer will use the /healthz endpoint to route traffic only to healthy instances.

Because each Droplet serves both the static frontend and the backend API, scaling the autoscale pool scales the entire application uniformly.

## Deployment Approach

Build the Go application into a single Linux binary and deploy it to each Droplet during initialization.

A startup script or custom image can:

1. Install the Go application binary.
2. Configure required environment variables, including the OpenRouter API key.
3. Configure the systemd service.
4. Start the Go service.
5. Expose the service on an internal port such as 8080.

This keeps the deployment simple while still allowing the infrastructure to scale horizontally.

## Security Considerations

The OpenRouter API key must be stored only on the server side, preferably as an environment variable available to the Go service.

The API key should never be embedded in:

- HTML
- CSS
- Browser-side JavaScript
- Public repositories
- Static frontend files

All requests from the browser should go to the Go server. The Go server is responsible for forwarding appropriate requests to OpenRouter.

## Recommendation

Proceed with a plain Go net/http application on Ubuntu Droplet Autoscale Pools behind a DigitalOcean Load Balancer.

The Go application should serve both the static frontend and the backend API. The static HTML, CSS, and JavaScript should be delivered by the Go server, and the browser-side JavaScript should send API requests to the Go backend endpoint, such as /api/chat.

This approach satisfies the key requirements:

- No Marketplace image
- No App Platform
- No Node.js
- No Python
- Secure handling of the OpenRouter key
- Simple frontend delivery
- Horizontally scalable production deployment
- One self-contained Go binary per Droplet