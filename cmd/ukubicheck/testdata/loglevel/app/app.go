// Package app writes through the log package: each use is refused, its Logger type is not.
package app

import (
	"log"
	l "log"
	"log/slog"
	"net/http"
)

var srv = http.Server{ErrorLog: slog.NewLogLogger(slog.Default().Handler(), slog.LevelWarn)}

func f(lg *log.Logger) {
	log.Printf("x")           // want `^log\.Printf: give the line its level with slog`
	log.Fatal("x")            // want `^log\.Fatal:`
	l.Println("x")            // want `^log\.Println:`
	p := log.Default().Printf // want `^log\.Default:` `^log\.Printf:`
	log.SetOutput(nil)        // want `^log\.SetOutput:`
	lg.Print("x")             // want `^log\.Print:`
	_ = log.LstdFlags         // want `^log\.LstdFlags:`
	var _ *log.Logger = srv.ErrorLog
	slog.Info("x")
	p("y")
}
