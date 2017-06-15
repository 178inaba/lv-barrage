package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"

	"github.com/178inaba/lv-barrage/nico"
	"github.com/howeyc/gopass"
	homedir "github.com/mitchellh/go-homedir"
)

const (
	defaultSessionFilePath = ".config/lv-barrage/token"
	hbIfseetnoComment      = "/hb ifseetno "
)

func main() {
	log.SetFlags(0)
	log.SetPrefix(fmt.Sprintf("%s: ", os.Args[0]))
	os.Exit(run())
}

func run() int {
	if len(os.Args) == 1 {
		fmt.Fprintf(os.Stderr, "usage: %s <live_id>\n", os.Args[0])
		return 1
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-sigCh
		cancel()
	}()

	_, err := getSessionFilePath()
	if err != nil {
		log.Print(err)
		return 1
	}

	c := nico.NewClient()
	mail, password, err := prompt()
	if err != nil {
		log.Print(err)
		return 1
	}
	_, err = c.Login(ctx, mail, password)
	if err != nil {
		log.Print(err)
		return 1
	}
	ps, err := c.GetPlayerStatus(ctx, os.Args[1])
	if err != nil {
		log.Print(err)
		return 1
	}
	ch, err := ps.Ms.StreamingComment(ctx, -100)
	if err != nil {
		log.Print(err)
		return 1
	}
	for ci := range ch {
		switch com := ci.(type) {
		case *nico.Chat:
			if strings.Contains(com.Comment, hbIfseetnoComment) {
				continue
			}
			fmt.Println(com.Comment)
		}
	}

	return 0
}

func getSessionFilePath() (string, error) {
	home, err := homedir.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, defaultSessionFilePath), nil
}

func prompt() (string, string, error) {
	// Login mail address from stdin.
	fmt.Print("Mail: ")
	var mail string
	fmt.Scanln(&mail)

	// Password from stdin.
	fmt.Print("Password: ")
	pBytes, err := gopass.GetPasswd()
	if err != nil {
		return "", "", err
	}

	return mail, string(pBytes), nil
}
