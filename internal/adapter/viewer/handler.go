package viewer

import (
	"net/http"
)

func HandlePage(w http.ResponseWriter, r *http.Request) {
	data, err := viewerFS.ReadFile("viewer.html")
	if err != nil {
		http.Error(w, "viewer page not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(data)
}

// MessageHandler processes a user message from the viewer.
