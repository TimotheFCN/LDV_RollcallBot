package main

import (
	"github.com/joho/godotenv"
	_ "github.com/joho/godotenv/autoload"
	"log"
	"os"
)

var login string
var password string

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	login := os.Getenv("S3_BUCKET")
	password := os.Getenv("SECRET_KEY")
}
