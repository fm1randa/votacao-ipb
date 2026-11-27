// Sistema de votação offline para Congresso de Federação UMP (IPB).
//
// Binário único: serve a app web e fala com um SQLite local (WAL).
// Roda numa LAN sem internet — notebook da mesa + roteador de viagem.
//
//	go run . -seed                    # cria dados de exemplo + tokens
//	go run .                          # sobe em http://0.0.0.0:8080
//	go run . -addr :9000 -pin 4729    # porta e PIN da mesa custom
package main

import (
	"errors"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"votacao-ipb/internal/cli"
)

func main() {
	stop := make(chan struct{})
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() { <-sig; close(stop) }()

	if err := cli.Run(os.Args[1:], stop); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		log.Fatal(err)
	}
}
