package main

import (
	"log"
	"time"

	"github.com/RoongChakkrin42/agnos-backend-assignment/internal/config"
	"github.com/RoongChakkrin42/agnos-backend-assignment/internal/database"
	"github.com/RoongChakkrin42/agnos-backend-assignment/internal/his"
	"github.com/RoongChakkrin42/agnos-backend-assignment/internal/server"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	db, err := database.Open(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("connect db: %v", err)
	}
	// a staff member is waiting on the other end: give up on a slow HIS after 5s and answer 502
	hisClient := his.NewClient(5 * time.Second)
	log.Fatal(server.New(db, cfg.JWTSecret, hisClient).Run(":" + cfg.Port))
}
