package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Overload(".env")

	if err := initDB(); err != nil {
		panic(err)
	}

	addr := fmt.Sprintf("%s:%s", os.Getenv("SERVER_IP"), os.Getenv("SERVER_PORT"))
	fmt.Println(fmt.Sprintf("Server listening on %s", addr))
	if err := http.ListenAndServe(addr, buildRouter()); err != nil {
		panic(err)
	}
}
