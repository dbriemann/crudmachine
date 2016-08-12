# crudmachine
Simple proof of concept for something specifically generic.. :)

# install
```
go get -u -v github.com/zensword/crudmachine
```

Start the demo by running `crudmachine` (port 8888 by default) or `crudmachine -p 1234` if you prefer a specific port.

Now you can play around with some generic crud stuff. See examples below.

The file `collections.conf` contains the names for all collections that will be created on startup.

# curl examples
### Create some books.
```
curl -X POST -H 'Content-Type: application/json' -d "{\"name\": \"book1\", \"isbn\": \"0815-1\"}" http://localhost:8888/db/books
curl -X POST -H 'Content-Type: application/json' -d "{\"name\": \"book2\", \"isbn\": \"0815-2\"}" http://localhost:8888/db/books
curl -X POST -H 'Content-Type: application/json' -d "{\"name\": \"book3\", \"isbn\": \"0815-3\"}" http://localhost:8888/db/books
curl -X POST -H 'Content-Type: application/json' -d "{\"name\": \"book4\", \"isbn\": \"0815-4\"}" http://localhost:8888/db/books
curl -X POST -H 'Content-Type: application/json' -d "{\"name\": \"book5\", \"isbn\": \"0815-5\"}" http://localhost:8888/db/books
```

### Retrieve all books.
```
curl -X GET http://localhost:8888/db/books
```

### Update a book. (use any id from last step)
Note that you can omit the id in the object itself. It will be reinserted.
```
curl -X PUT -H 'Content-Type: application/json' -d "{\"name\": \"updatedBook\", \"isbn\": \"0815-5\"}" http://localhost:8888/db/books/23453344545
```

### Delete a book. (id again..)
```
curl -X DELETE http://localhost:8888/db/books/23453344545
```