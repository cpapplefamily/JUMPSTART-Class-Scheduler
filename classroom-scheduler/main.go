package main

import (
	"fmt"
	"log"
	"net/http"
	"scheduler/web"
)

const httpPort = 8080

func main() {
	// 1. Database + cache
	web.OpenDBAndLoadCaches()

	// 2. Templates
	//web.InitTemplates()

	// 3. All routes are now owned by the web package
	web.SetupRoutes()

	log.Printf("    http://localhost:%d", httpPort)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", httpPort), web.LoggingMiddleware(http.DefaultServeMux)))
}