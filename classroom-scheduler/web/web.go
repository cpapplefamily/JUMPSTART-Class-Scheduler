// web/server.go
package web

import "net/http"

func SetupRoutes() {
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	// Core pages
	http.HandleFunc("/", IndexHandler)
	http.HandleFunc("/classroom/", ClassroomHandler)
	http.HandleFunc("/config", ConfigHandler)
	http.HandleFunc("/config/save", ConfigSaveHandler)
	http.HandleFunc("/blocks", BlocksHandler)
	http.HandleFunc("/blocks/save", BlocksSaveHandler)
}