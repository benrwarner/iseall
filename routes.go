package main

import "net/http"

func registerRoutes(mux *http.ServeMux, h *hub) {
	mux.HandleFunc("GET /api/auth", handleAuthStatus)
	mux.HandleFunc("POST /api/auth/setup", handleSetup)
	mux.HandleFunc("POST /api/auth/login", handleLogin)
	mux.HandleFunc("POST /api/auth/logout", handleLogout)

	a := func(pattern string, fn http.HandlerFunc) { mux.Handle(pattern, requireAuth(fn)) }
	a("POST /api/auth/password", handleChangePassword)
	a("GET /api/settings", handleSettings)
	a("POST /api/settings", handleSettings)

	a("GET /api/stream", h.serveSSE)
	a("GET /api/procs", h.handleProcs)
	a("GET /api/procs/{pid}", handleProcDetail)
	a("POST /api/procs/signal", handleSignal)
	a("GET /api/ports", handlePorts)
	a("GET /api/trends", h.handleTrends)

	a("GET /api/docker", handleDocker)
	a("GET /api/docker/images", handleDockerImages)
	a("POST /api/docker/images/remove", handleDockerImageRemove)
	a("POST /api/docker/prune", handleDockerPrune)
	a("POST /api/docker/stack", handleStack)
	a("GET /api/docker/c/{id}", handleDockerInspect)
	a("GET /api/docker/c/{id}/logs", handleDockerLogs)
	a("POST /api/docker/c/{id}/{action}", handleDockerAction)

	a("GET /api/services", handleServices)
	a("GET /api/services/{unit}", handleServiceDetail)
	a("POST /api/services/{unit}/{action}", handleServiceAction)
	a("GET /api/logs", handleJournal)
	a("GET /api/schedule", handleSchedule)

	a("GET /api/boot", handleBoot)
	a("POST /api/startup", handleStartupSave)
	a("POST /api/startup/{name}/delete", handleStartupDelete)

	a("GET /api/network", handleNetwork)
	a("GET /api/storage", handleStorage)
	a("GET /api/security", handleSecurity)
	a("GET /api/updates", handleUpdates)
	a("POST /api/power", handlePower)
	a("GET /api/term", handleTerm)
}
