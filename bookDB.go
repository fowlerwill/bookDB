package main

import (
	"database/sql"
	"flag"
	"github.com/coopernurse/gorp"
	_ "github.com/mattn/go-sqlite3"
	"html/template"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var (
	addr = flag.Bool("addr", false, "find open address and print to final-port.txt")
)

type Page struct {
	Title string
	Body  []byte
}

// Saves a page to a txt - must convert to save to db
func (p *Page) save() error {
	filename := p.Title + ".txt"
	return ioutil.WriteFile(filename, p.Body, 0600)
}

// Loads a page from an existing .txt - must be converted to load from db
func loadPage(title string) (*Page, error) {

	//Right here - make this query the DB for a book.
	filename := title + ".txt"
	body, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	return &Page{Title: title, Body: body}, nil
}

// = Begin Handler functions for the routes
// ---------------------------------------------

func makeHandler(fn func(http.ResponseWriter, *http.Request, string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		m := validPath.FindStringSubmatch(r.URL.Path)
		if m == nil {
			http.NotFound(w, r)
			return
		}
		fn(w, r, m[2])
	}
}

/**
 * A function to handle a loop to run through every book in
 * the database.
 *
 * Currently searches for .txt files - should be changed
 * 	to use the database.
 */
func loopHandler(w http.ResponseWriter, r *http.Request) {

	// fetch all rows
	var books []Book
	_, err := dbmap.Select(&books, "select * from books order by book_id")
	checkErr(err, "Select failed")
	log.Println("All rows:")
	for x, p := range books {
		log.Printf("    %d: %v\n", x, p)
	}

	matches, err := filepath.Glob("*.txt")
	if err != nil {
		return
	}
	for _, s := range matches {
		s = strings.Replace(s, ".txt", "", 1)
		p, err := loadPage(s)

		if err == nil {
			renderTemplate(w, "view", p)
		}
	}
}

func bookViewHandler(w http.ResponseWriter, r *http.Request, theBook Book) {
	//p, err := loadBook(title)
	return
}

func viewHandler(w http.ResponseWriter, r *http.Request, title string) {
	p, err := loadPage(title)
	if err != nil {
		http.Redirect(w, r, "/edit/"+title, http.StatusFound)
		return
	}
	renderTemplate(w, "view", p)
}

func editHandler(w http.ResponseWriter, r *http.Request, title string) {
	p, err := loadPage(title)
	if err != nil {
		p = &Page{Title: title}
	}
	renderTemplate(w, "edit", p)
}

func saveHandler(w http.ResponseWriter, r *http.Request, title string) {
	body := r.FormValue("body")
	p := &Page{Title: title, Body: []byte(body)}
	err := p.save()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/view/"+title, http.StatusFound)
}

// prevents code injection by parsing ONLY html & escaped Go
var templates = template.Must(template.ParseFiles("edit.html", "view.html"))

func renderTemplate(w http.ResponseWriter, tmpl string, p *Page) {
	err := templates.ExecuteTemplate(w, tmpl+".html", p)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// checks for valid URLs
var validPath = regexp.MustCompile("^/(edit|save|view)/([a-zA-Z0-9]+)$")

// initialize the DbMap
var dbmap = initDb()

// A main method to run this gaffer.
func main() {
	defer dbmap.Db.Close()

	// Routes!
	http.HandleFunc("/view/", makeHandler(viewHandler))
	http.HandleFunc("/edit/", makeHandler(editHandler))
	http.HandleFunc("/save/", makeHandler(saveHandler))
	http.HandleFunc("/", loopHandler)

	// = Begin database demo stuff
	// ---------------------------------------------
	// delete any existing rows
	err := dbmap.TruncateTables()
	checkErr(err, "TruncateTables failed")

	// create two posts
	g := newGenre(1, "Fantasy")
	b := newBook(1, "Game of Thrones", 1, "12345", 10000, 1, true, 1, 1, 1, 1)
	p1 := newPost("Go 1.1 released!", "Lorem ipsum lorem ipsum")
	p2 := newPost("Go 1.2 released!", "Lorem ipsum lorem ipsum")

	// insert rows - auto increment PKs will be set properly after the insert
	err = dbmap.Insert(&p1, &p2, &g, &b)
	checkErr(err, "Insert failed")

	count1, err1 := dbmap.SelectInt("select count(*) from genres")
	checkErr(err1, "select count(*) failed")
	log.Println("Rows after inserting into genres:", count1)

	// use convenience SelectInt
	count, err := dbmap.SelectInt("select count(*) from posts")
	checkErr(err, "select count(*) failed")
	log.Println("Rows after inserting:", count)

	// update a row
	p2.Title = "Go 1.2 is better than ever"
	count, err = dbmap.Update(&p2)
	checkErr(err, "Update failed")
	log.Println("Rows updated:", count)

	// fetch one row - note use of "post_id" instead of "Id" since column is aliased
	//
	// Postgres users should use $1 instead of ? placeholders
	// See 'Known Issues' below
	//
	err = dbmap.SelectOne(&g, "select * from genres where genre_id=?", g.Id)
	checkErr(err, "SelectOne failed")
	log.Println("p2 row:", g)

	// fetch all rows
	var posts []Post
	_, err = dbmap.Select(&posts, "select * from posts order by post_id")
	checkErr(err, "Select failed")
	log.Println("All rows:")
	for x, p := range posts {
		log.Printf("    %d: %v\n", x, p)
	}

	// delete row by PK
	count, err = dbmap.Delete(&p1)
	checkErr(err, "Delete failed")
	log.Println("Rows deleted:", count)

	// delete row manually via Exec
	_, err = dbmap.Exec("delete from posts where post_id=?", p2.Id)
	checkErr(err, "Exec failed")

	// confirm count is zero
	count, err = dbmap.SelectInt("select count(*) from posts")
	checkErr(err, "select count(*) failed")
	log.Println("Row count - should be zero:", count)

	log.Println("Done!")

	// = end database demo stuff !

	// This bit seems to start the server
	if *addr {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			log.Fatal(err)
		}
		err = ioutil.WriteFile("final-port.txt", []byte(l.Addr().String()), 0644)
		if err != nil {
			log.Fatal(err)
		}
		s := &http.Server{}
		s.Serve(l)
		return
	}

	http.ListenAndServe(":8080", nil)
}

func newPost(title, body string) Post {
	return Post{
		Created: time.Now().UnixNano(),
		Title:   title,
		Body:    body,
	}
}

func initDb() *gorp.DbMap {
	// connect to db using standard Go database/sql API
	// use whatever database/sql driver you wish
	db, err := sql.Open("sqlite3", "/tmp/post_db.bin")
	checkErr(err, "sql.Open failed")

	// construct a gorp DbMap
	dbmap := &gorp.DbMap{Db: db, Dialect: gorp.SqliteDialect{}}

	// add a table, setting the table name to 'posts' and
	// specifying that the Id property is an auto incrementing PK
	dbmap.AddTableWithName(Post{}, "posts").SetKeys(true, "Id")
	dbmap.AddTableWithName(Book{}, "books").SetKeys(true, "Id")
	dbmap.AddTableWithName(Author{}, "authors").SetKeys(true, "Id")
	dbmap.AddTableWithName(Genre{}, "genres").SetKeys(true, "Id")
	dbmap.AddTableWithName(Publisher{}, "publishers").SetKeys(true, "Id")
	dbmap.AddTableWithName(Series{}, "series").SetKeys(true, "Id")
	dbmap.AddTableWithName(Language{}, "languages").SetKeys(true, "Id")

	// create the table. in a production system you'd generally
	// use a migration tool, or create the tables via scripts
	err = dbmap.CreateTablesIfNotExists()
	checkErr(err, "Create tables failed")

	return dbmap
}

func checkErr(err error, msg string) {
	if err != nil {
		log.Fatalln(msg, err)
	}
}

// = Book database specific structs
// ------------------------------------------------
type Book struct {
	Id           int `db:"book_id"`
	Name         string
	Author_id    int
	Isbn         string
	Pubdate      int
	Edition      int
	Isfiction    bool
	Genre_id     int
	Publisher_id int
	Series_id    int
	Language_id  int
}

func newBook(id int, aName string, authorId int, isbn string, pubdate int, edition int, isFiction bool, genreId int, publisherId int, seriesId int, languageId int) Book {
	return Book{
		Id:           id,
		Name:         aName,
		Author_id:    authorId,
		Isbn:         isbn,
		Pubdate:      pubdate,
		Edition:      edition,
		Isfiction:    isFiction,
		Genre_id:     genreId,
		Publisher_id: publisherId,
		Series_id:    seriesId,
		Language_id:  languageId,
	}
}

type Author struct {
	Id         int64 `db:"author_id"`
	firstname  string
	lastname   string
	pseudonyms string
}

type Genre struct {
	Id   int `db:"genre_id"`
	Name string
}

func newGenre(id int, aName string) Genre {
	return Genre{
		Id:   id,
		Name: aName,
	}
}

type Publisher struct {
	Id   int64 `db:"publisher_id"`
	name string
}

type Series struct {
	Id   int64 `db:"series_id"`
	name string
}

type Language struct {
	Id   int64 `db:"language_id"`
	name string
}

type Post struct {
	// db tag lets you specify the column name if it differs from the struct field
	Id      int64 `db:"post_id"`
	Created int64
	Title   string
	Body    string
}
