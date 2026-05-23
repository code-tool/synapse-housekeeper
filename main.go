package main

import (
	_ "github.com/joho/godotenv/autoload"

	"synapse-housekeeper/cmd"
)

func main() {
	cmd.Execute()
}
