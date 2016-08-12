package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/HouzuoGuo/tiedot/db"
	"goji.io"
	"goji.io/pat"
	"golang.org/x/net/context"
)

const (
	DBFolder          = "storage"
	CollectionsConfig = "collections.conf"
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
// This is thread-safe thanks to Tiedot.
func NewDBController(db *db.DB) *DBController {
	c := &DBController{
		DB: db,
	}
	return c
}

// SetupCollections reads all collection names from the config file
// and creates the collections in the database if they don't exist yet.
// This should be run at startup.
func (d *DBController) SetupCollections(cfgFilePath string) {
	fmt.Println("Reading collections from file and creating them in DB.")
	// Read collections config file. Every line contains one collection name.
	// Only a-z,A-Z allowed.
	file, err := os.Open(CollectionsConfig)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	allCollections := d.DB.AllCols()
	fmt.Println("Current collections in DB", allCollections)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		// Check collection name for validity.
		collName := scanner.Text()
		collName = strings.TrimSpace(collName)
		re := regexp.MustCompile("^[a-zA-Z]*$")

		if !re.MatchString(collName) {
			panic(fmt.Errorf("Collection name '%s' has invalid characters", collName))
		}

		create := true

		// Create collection if it does not exist.
		for _, c := range allCollections {
			if collName == c {
				create = false
			}
		}

		if create {
			fmt.Println("Creating collection", collName)
			if err := d.DB.Create(collName); err != nil {
				panic(err)
			}

			allCollections = append(allCollections, collName)
		} else {
			fmt.Printf("skipping '%s': already exists\n", collName)
		}
	}

	if err := scanner.Err(); err != nil {
		panic(err)
	}

	// TODO maybe remove unused collections, which are not included in config file
	// but exist in database?
}

// CreateDocumentHandler handles: POST /db/:collection.
// A new arbitrary entry is created in the 'collection'.
// If the collection does not exist it is created.
func (d *DBController) CreateDocumentHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	// Parse collection type from path.
	collName := pat.Param(ctx, "collection")
	fmt.Println("collection:", collName)

	coll := d.DB.Use(collName)
	if coll == nil {
		WriteResponse(ctx, w, http.StatusInternalServerError, map[string]interface{}{
			"error": "could not use collection " + collName,
		})
		return
	}

	// Parse JSON object from POST parameter.
	js, err := ParsePostJSON(r)
	if err != nil {
		WriteResponse(ctx, w, http.StatusBadRequest, map[string]interface{}{
			"error": "request body does not contain valid json: " + err.Error(),
		})
		return
	}

	// Insert object into collection.
	docID, err := coll.Insert(js)
	if err != nil {
		WriteResponse(ctx, w, http.StatusInternalServerError, map[string]interface{}{
			"error": "could not insert document: " + err.Error(),
		})
		return
	}

	// Read it back to add id to document.
	readBack, err := coll.Read(docID)
	if err != nil {
		WriteResponse(ctx, w, http.StatusInternalServerError, map[string]interface{}{
			"error": "could not insert document: " + err.Error(),
		})
		return
	}

	readBack["id"] = strconv.Itoa(docID)

	if err := coll.Update(docID, readBack); err != nil {
		WriteResponse(ctx, w, http.StatusInternalServerError, map[string]interface{}{
			"error": "could not add id to document: " + err.Error(),
		})
		return
	}

	fmt.Println("created document:", readBack)

	// Everything done. Return document.
	WriteResponse(ctx, w, http.StatusCreated, readBack)
}

// ReadCollectionHandler handles: GET /db/:collection.
// Return all documents contained in the given collection.
func (d *DBController) ReadCollectionHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	collName := pat.Param(ctx, "collection")
	result, err := d.Search(collName, "all")
	if err != nil {
		WriteResponse(ctx, w, http.StatusInternalServerError, map[string]interface{}{
			"error": "could not read from collection " + collName,
		})
		return
	}

	// Respond with results
	WriteResponse(ctx, w, http.StatusOK, result)
}

// Search searches the given collection with the given tiedot query string and
// returns all results that satisfy the query data.
func (d *DBController) Search(collection string, query interface{}) (map[string]interface{}, error) {
	queryResult := make(map[int]struct{})
	result := map[string]interface{}{}
	temp := []interface{}{}

	coll := d.DB.Use(collection)
	if coll == nil {
		return result, fmt.Errorf("could not use collection")
	}

	if err := db.EvalQuery(query, coll, &queryResult); err != nil {
		return result, err
	}

	// Query result are document IDs.
	for id := range queryResult {
		// To get query result document, simply read it
		readBack, err := coll.Read(id)
		if err != nil {
			return result, err
		}
		temp = append(temp, readBack)
	}

	result["results"] = temp

	return result, nil
}

// ReadDocumentHandler queries the given collection for a given id
// and serves the found document if it exists.
func (d *DBController) ReadDocumentHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	collName := pat.Param(ctx, "collection")
	strid := pat.Param(ctx, "id")

	id, err := strconv.Atoi(strid)
	if err != nil {
		WriteResponse(ctx, w, http.StatusBadRequest, map[string]interface{}{
			"error": "id cannot be parsed to number",
		})
		return
	}

	coll := d.DB.Use(collName)
	if coll == nil {
		WriteResponse(ctx, w, http.StatusInternalServerError, map[string]interface{}{
			"error": "could not use collection " + collName,
		})
		return
	}

	result, err := coll.Read(id)
	if err != nil {
		WriteResponse(ctx, w, 422, map[string]interface{}{
			"error": "document not found",
		})
		return
	}

	WriteResponse(ctx, w, http.StatusOK, result)
}

// UpdateDocumentHandler queries the given collection for a given id
// and updates the found document with the payload json data.
func (d *DBController) UpdateDocumentHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	collName := pat.Param(ctx, "collection")
	strid := pat.Param(ctx, "id")

	id, err := strconv.Atoi(strid)
	fmt.Println(strid, id)
	if err != nil {
		WriteResponse(ctx, w, http.StatusBadRequest, map[string]interface{}{
			"error": "id cannot be parsed to number",
		})
		return
	}

	coll := d.DB.Use(collName)
	if coll == nil {
		WriteResponse(ctx, w, http.StatusInternalServerError, map[string]interface{}{
			"error": "could not use collection " + collName,
		})
		return
	}

	// Parse JSON object from POST parameter.
	js, err := ParsePostJSON(r)
	if err != nil {
		WriteResponse(ctx, w, http.StatusBadRequest, map[string]interface{}{
			"error": "request body does not contain valid json: " + err.Error(),
		})
		return
	}

	// Always replace id with correct id == avoid user errors.
	js["id"] = strconv.Itoa(id)

	if err = coll.Update(id, js); err != nil {
		WriteResponse(ctx, w, http.StatusInternalServerError, map[string]interface{}{
			"error": "could not update document",
		})
		return
	}

	// Update successful
	WriteResponse(ctx, w, http.StatusOK, js)
}

// DeleteDocumentHandler deletes document with given id from given collection.
func (d *DBController) DeleteDocumentHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	collName := pat.Param(ctx, "collection")
	strid := pat.Param(ctx, "id")

	id, err := strconv.Atoi(strid)
	fmt.Println(strid, id)
	if err != nil {
		WriteResponse(ctx, w, http.StatusBadRequest, map[string]interface{}{
			"error": "id cannot be parsed to number",
		})
		return
	}

	coll := d.DB.Use(collName)
	if coll == nil {
		WriteResponse(ctx, w, http.StatusInternalServerError, map[string]interface{}{
			"error": "could not use collection " + collName,
		})
		return
	}

	if err := coll.Delete(id); err != nil {
		WriteResponse(ctx, w, http.StatusInternalServerError, map[string]interface{}{
			"error": "could not delete document with id " + strid,
		})
		return
	}

	WriteResponse(ctx, w, http.StatusOK, map[string]interface{}{
		"id": strid,
	})
}

// SearchCollectionHandler handles: POST /db/search/:collection.
// Return all documents contained in the given collection fulfilling the query properties.
// Expects a Tiedot query string. See: https://github.com/HouzuoGuo/tiedot/wiki/Query-processor-and-index
// Payload example:
// {
//	 "query": "[{"eq": "JohnAppleseed", "in": ["username"], "limit": 1}]"
//}
// TODO
func (d *DBController) SearchCollectionHandler(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	// Parse JSON object from POST parameter.
	//	jsonQuery, err := ParsePostJSON(r)
	//	if err != nil {
	//		WriteResponse(ctx, w, http.StatusBadRequest, map[string]interface{}{
	//			"error": "request body does not contain valid json: " + err.Error(),
	//		})
	//		return
	//	}

	// sorry no more time for now..
}

func main() {
	// Read command line flags.
	var port int
	flag.IntVar(&port, "p", 8888, "specify port to use")
	flag.Parse()

	// Create folder if it doesn't exist.
	DB, err := db.OpenDB(DBFolder)
	if err != nil {
		panic(err)
	}

	dbController := NewDBController(DB)

	dbController.SetupCollections(CollectionsConfig)
	fmt.Println("..done creating collections.")

	// Create http router.
	mux := goji.NewMux()

	// And assign all the crud routes to the handler methods.
	mux.HandleFuncC(pat.Get("/db/:collection"), dbController.ReadCollectionHandler)

	mux.HandleFuncC(pat.Post("/db/:collection"), dbController.CreateDocumentHandler)
	mux.HandleFuncC(pat.Get("/db/:collection/:id"), dbController.ReadDocumentHandler)
	mux.HandleFuncC(pat.Put("/db/:collection/:id"), dbController.UpdateDocumentHandler)
	mux.HandleFuncC(pat.Delete("/db/:collection/:id"), dbController.DeleteDocumentHandler)

	// TODO this method still needs implementation..
	mux.HandleFuncC(pat.Post("/db/search/:collection"), dbController.SearchCollectionHandler)

	// Start http server.
	fmt.Println("Listening on localhost:", port)
	http.ListenAndServe("localhost:"+strconv.Itoa(port), mux)
}
