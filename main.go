package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Todo struct {
	ID        primitive.ObjectID `bson:"_id,omitempty"`
	Title     string             `bson:"title"`
	Done      bool               `bson:"done"`
	CreatedAt time.Time          `bson:"created_at"`
}

type TodoJSON struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Done      bool      `json:"done"`
	CreatedAt time.Time `json:"created_at"`
}

func toJSON(t Todo) TodoJSON {
	return TodoJSON{
		ID:        t.ID.Hex(),
		Title:     t.Title,
		Done:      t.Done,
		CreatedAt: t.CreatedAt,
	}
}

var coll *mongo.Collection

func main() {
	// 1) Mongo bağlan
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		log.Fatal("mongo connect:", err)
	}
	// basit ping
	if err := client.Ping(ctx, nil); err != nil {
		log.Fatal("mongo ping:", err)
	}
	coll = client.Database("gotodo").Collection("todos")
	log.Println("MongoDB connected")

	// 2) HTTP uçları
	http.HandleFunc("/todos", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			listTodos(w, r)
		case http.MethodPost:
			createTodo(w, r)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	log.Println("listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

// GET /todos  (+ opsiyonel ?done=true|false)
func listTodos(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	filter := bson.D{}
	if q := r.URL.Query().Get("done"); q != "" {
		switch strings.ToLower(q) {
		case "true", "1", "t", "on":
			filter = bson.D{{Key: "done", Value: true}}
		case "false", "0", "f", "off":
			filter = bson.D{{Key: "done", Value: false}}
		default:
			http.Error(w, `{"error":"done true/false olmalı"}`, http.StatusBadRequest)
			return
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	cur, err := coll.Find(ctx, filter)
	if err != nil {
		http.Error(w, `{"error":"db find hatası"}`, http.StatusInternalServerError)
		return
	}
	defer cur.Close(ctx)

	var docs []Todo
	if err := cur.All(ctx, &docs); err != nil {
		http.Error(w, `{"error":"db decode hatası"}`, http.StatusInternalServerError)
		return
	}

	out := make([]TodoJSON, 0, len(docs))
	for _, d := range docs {
		out = append(out, toJSON(d))
	}
	_ = json.NewEncoder(w).Encode(out)
}

// POST /todos   body: {"title":"..."}
func createTodo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	var req struct {
		Title string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Title) == "" {
		http.Error(w, `{"error":"geçersiz JSON ya da boş title"}`, http.StatusBadRequest)
		return
	}

	doc := Todo{
		Title:     strings.TrimSpace(req.Title),
		Done:      false,
		CreatedAt: time.Now(),
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	res, err := coll.InsertOne(ctx, doc)
	if err != nil {
		http.Error(w, `{"error":"db insert hatası"}`, http.StatusInternalServerError)
		return
	}
	if oid, ok := res.InsertedID.(primitive.ObjectID); ok {
		doc.ID = oid
	}

	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(toJSON(doc))
}
