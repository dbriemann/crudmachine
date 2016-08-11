package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"strconv"

	"github.com/HouzuoGuo/tiedot/db"
	shortid "github.com/ventu-io/go-shortid"
	"goji.io"
	"goji.io/pat"
	"golang.org/x/net/context"
)

const (
	DBFolder = "storage"
)

// WriteResponse writes the resp interface with assigned http status code as JSON response
// to the given http.ResponseWriter.
func WriteResponse(ctx context.Context, w http.ResponseWriter, status int, resp interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(resp); err != nil {
		fmt.Println("Error writing json response:", err.Error())
	}
}

// ParsePostJSON parses the request body from a POST request and
// returns the decoded JSON as map[string]interface{}.
func ParsePostJSON(r *http.Request) (map[string]interface{}, error) {
	ret := map[string]interface{}{}

	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(&ret)

	return ret, err
}

// DBController is a helper struct to hold a db instance for handler methods.
type DBController struct {
	DB *db.DB
}

// NewDBController creates an instance of DBController with a pointer to the given database.
// This is threadsafe thanks to Tiedot.
func NewDBController(db *db.DB) *DBController {
	c := &DBController{
		DB: db,
	}
	return c
}

// Tries to acquire a collection from DB and creates it if it does not exist.
func (d *DBController) UseCollection(name string) (*db.Col, error) {
	// Try to use(open) collection.
	coll := d.DB.Use(name)
	if coll == nil {
		// Collection does not exist -> create it.
		if err := d.DB.Create(name); err != nil {
			return coll, err
		}

		// Create index for id attribute for every new collection.
		// It is used for shortIDs.
		if err := coll.Index([]string{"id"}); err != nil {
			panic(err)
		}
	}

	return coll, nil
}

// CreateCollectionHandler handles: POST /db/:collection.
// A new arbitrary entry is created in the 'collection'.
// If the collection does not exist it is created.
func (d *DBController) CreateCollectionHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	// Parse collection type from path.
	collName := pat.Param(ctx, "collection")
	fmt.Println("collection:", collName)

	coll, err := d.UseCollection(collName)
	if err != nil {
		WriteResponse(ctx, w, http.StatusInternalServerError, map[string]interface{}{
			"error": "could not create collection " + collName,
		})
		return
	}
	return

	// Parse JSON object from POST parameter.
	js, err := ParsePostJSON(r)
	if err != nil {
		WriteResponse(ctx, w, http.StatusBadRequest, map[string]interface{}{
			"error": "request body does not contain valid json: " + err.Error(),
		})
		return
	}

	// Generate unique short id for the object.
	sid, err := shortid.Generate()
	if err != nil {
		WriteResponse(ctx, w, http.StatusInternalServerError, map[string]interface{}{
			"error": "could not generate unique id",
		})
		return
	}

	js["id"] = sid

	// Insert object into collection. Discard db id since we use extra indexed shortIDs.
	if _, err := coll.Insert(js); err != nil {
		WriteResponse(ctx, w, http.StatusInternalServerError, map[string]interface{}{
			"error": "could not insert document: " + err.Error(),
		})
		return
	}

	// Everything done. Return document.
	WriteResponse(ctx, w, http.StatusOK, js)
}

func main() {
	// Init short id library.
	// NOTE: The package guarantees the generation of unique Ids with no collisions for 34 years
	// (1/1/2016-1/1/2050) using the same worker Id within a single (although can be concurrent)
	// application provided application restarts take longer than 1 millisecond.
	customABC := "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ_#"
	sid, err := shortid.New(1, customABC, 1)
	if err != nil {
		panic(err)
	}
	shortid.SetDefault(sid)

	// Read command line flags.
	var port int
	flag.IntVar(&port, "p", 8888, "specify port to use")
	flag.Parse()

	fmt.Println("Running on localhost:", port)

	// Create folder if it doesn't exist.
	DB, err := db.OpenDB(DBFolder)
	if err != nil {
		panic(err)
	}

	dbController := NewDBController(DB)

	// Create http router.
	mux := goji.NewMux()
	// And assign all the crud routes to the handler methods.
	mux.HandleFuncC(pat.Post("/db/:collection"), dbController.CreateCollectionHandler)

	// Start http server.
	http.ListenAndServe("localhost:"+strconv.Itoa(port), mux)
}
