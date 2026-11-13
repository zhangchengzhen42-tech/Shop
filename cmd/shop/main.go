package main

import (
	"flag"
	"log"
	"net/http"
	"os"

	"github.com/zhangchengzhen42-tech/Shop/internal/app"
	"github.com/zhangchengzhen42-tech/Shop/internal/web"
)

func main() {
	if err := run(os.Args[1:], os.Getenv, http.ListenAndServe); err != nil {
		log.Fatal(err)
	}

}

func run(args []string, getenv func(string) string, listen func(string, http.Handler) error) error {
	flags := flag.NewFlagSet("shop", flag.ContinueOnError)
	address := flags.String("addr", ":8080", "HTTP listen address")
	database := flags.String("db", "shop.db", "SQLite database path")
	if err := flags.Parse(args); err != nil {
		return err
	}
	server, store, err := newApplication(*database, getenv("SHOP_COOKIE_SECRET"))
	if err != nil {
		return err
	}
	defer store.Close()
	log.Printf("Shop 交易系统运行于 http://localhost%s", *address)
	return listen(*address, server.Handler())
}

func newApplication(database, secret string) (*web.Server, *app.Store, error) {
	store, err := app.Open(database)
	if err != nil {
		return nil, nil, err
	}
	if secret == "" {
		secret = "local-demo-only-change-in-production"
	}
	return web.New(store, secret), store, nil
}
