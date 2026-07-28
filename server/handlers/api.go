package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"db/server/internal/db"
)

var database *db.Database

func Init(d *db.Database) {
	database = d
}

// validCollectionName rejects empty, dot-prefixed, and path-traversal names.
func validCollectionName(name string) bool {
	if name == "" {
		return false
	}
	if strings.HasPrefix(name, ".") {
		return false
	}
	if strings.ContainsAny(name, "/\\") {
		return false
	}
	if strings.Contains(name, "..") {
		return false
	}
	// max length guard
	if len(name) > 128 {
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func CollectionsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		colls, err := database.ListCollections()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, colls)
	case http.MethodPost:
		var req struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if !validCollectionName(req.Name) {
			http.Error(w, "invalid collection name", http.StatusBadRequest)
			return
		}
		_, err := database.GetCollection(req.Name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func CollectionHandler(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/api/collections/")
	if name == "" || !validCollectionName(name) {
		http.Error(w, "invalid collection name", http.StatusBadRequest)
		return
	}
	if r.Method == http.MethodDelete {
		if err := database.DropCollection(name); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

func InsertDocumentHandler(w http.ResponseWriter, r *http.Request) {
	name := extractCollectionName(r.URL.Path)
	coll, err := database.GetCollection(name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	var doc map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&doc); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	id, err := coll.Insert(doc)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]string{"_id": id})
}

func InsertManyDocumentsHandler(w http.ResponseWriter, r *http.Request) {
	name := extractCollectionName(r.URL.Path)
	coll, err := database.GetCollection(name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	var req struct {
		Documents []map[string]interface{} `json:"documents"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ids, err := coll.InsertMany(req.Documents)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]interface{}{"ids": ids, "inserted": len(ids)})
}

func GetDocumentHandler(w http.ResponseWriter, r *http.Request) {
	name, id := extractCollectionAndID(r.URL.Path)
	coll, err := database.GetCollection(name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	doc, err := coll.FindByID(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, doc)
}

func UpdateDocumentHandler(w http.ResponseWriter, r *http.Request) {
	name, id := extractCollectionAndID(r.URL.Path)
	coll, err := database.GetCollection(name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	var update map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := coll.Update(id, update); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func DeleteDocumentHandler(w http.ResponseWriter, r *http.Request) {
	name, id := extractCollectionAndID(r.URL.Path)
	coll, err := database.GetCollection(name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if err := coll.Delete(id); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func FindDocumentsHandler(w http.ResponseWriter, r *http.Request) {
	name := extractCollectionName(r.URL.Path)
	coll, err := database.GetCollection(name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	var raw map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if raw == nil {
		raw = map[string]interface{}{}
	}
	filter, opt := db.ParseFindRequest(raw)
	docs, next, err := coll.FindPage(filter, opt)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if docs == nil {
		docs = []map[string]interface{}{}
	}
	// Always wrap so clients can read nextCursor; array-only clients still
	// work via the Documents field in SDKs that accept both shapes.
	writeJSON(w, map[string]interface{}{
		"documents":  docs,
		"nextCursor": next,
	})
}

func CreateIndexHandler(w http.ResponseWriter, r *http.Request) {
	name := extractCollectionName(r.URL.Path)
	coll, err := database.GetCollection(name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	var req struct {
		Field string `json:"field"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := coll.CreateIndex(req.Field); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func ListIndexesHandler(w http.ResponseWriter, r *http.Request) {
	name := extractCollectionName(r.URL.Path)
	coll, err := database.GetCollection(name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, coll.ListIndexes())
}

func DropIndexHandler(w http.ResponseWriter, r *http.Request) {
	name, field := extractCollectionAndIndexField(r.URL.Path)
	if field == "" {
		http.Error(w, "index field required", http.StatusBadRequest)
		return
	}
	coll, err := database.GetCollection(name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if err := coll.DropIndex(field); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func CollectionStatsHandler(w http.ResponseWriter, r *http.Request) {
	name := extractCollectionName(r.URL.Path)
	coll, err := database.GetCollection(name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	st, err := coll.StatsDetailed()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]interface{}{
		"count":       st.Docs,
		"storageSize": st.Bytes,
		"segments":    st.Segments,
		"indexes":     st.Indexes,
		"syncMode":    st.SyncMode,
		"docCache":    st.DocCache,
	})
}

func extractCollectionName(path string) string {
	parts := strings.Split(path, "/")
	for i, p := range parts {
		if p == "collections" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

func extractCollectionAndID(path string) (string, string) {
	parts := strings.Split(path, "/")
	for i, p := range parts {
		if p == "collections" && i+1 < len(parts) {
			collection := parts[i+1]
			if i+3 < len(parts) && parts[i+2] == "docs" {
				id := parts[i+3]
				return collection, id
			}
		}
	}
	return "", ""
}

func extractCollectionAndIndexField(path string) (string, string) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	for i, p := range parts {
		if p == "collections" && i+3 < len(parts) && parts[i+2] == "indexes" {
			return parts[i+1], parts[i+3]
		}
	}
	return "", ""
}
