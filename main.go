package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/178inaba/lv-barrage/nico"
	"github.com/howeyc/gopass"
	homedir "github.com/mitchellh/go-homedir"
)

const (
	defaultSessionFilePath = ".config/lv-barrage/session"
	hbIfseetnoComment      = "/hb ifseetno "
)

var (
	isAnonymous = flag.Bool("a", false, "Post anonymous user (184)")
	isPostOnce  = flag.Bool("o", false, "Post once")
)

func main() {
	log.SetFlags(0)
	log.SetPrefix(fmt.Sprintf("%s: ", os.Args[0]))
	flag.Parse()
	os.Exit(run())
}

func run() int {
	args := flag.Args()
	if len(args) != 2 {
		fmt.Fprintf(os.Stderr, "usage: %s <live_id> <comment>\n", os.Args[0])
		return 1
	}
	liveID := args[0]
	comment := args[1]

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-sigCh
		cancel()
	}()

	sessionFilePath, err := getSessionFilePath()
	if err != nil {
		log.Print(err)
		return 1
	}

	c := nico.NewClient()
	userSession, err := getSession(sessionFilePath)
	if err != nil || userSession == "" {
		mail, password, err := prompt()
		if err != nil {
			log.Print(err)
			return 1
		}
		userSession, err = c.Login(ctx, mail, password)
		if err != nil {
			log.Print(err)
			return 1
		}

		if err := saveSession(userSession, sessionFilePath); err != nil {
			log.Print(err)
			return 1
		}
	} else {
		c.UserSession = userSession
	}

	continueDuration := 10 * time.Second
	var mail nico.Mail
	if *isAnonymous {
		mail.Is184 = true
	}
	for {
		select {
		case <-ctx.Done():
			// Signal interrupt.
			return 0
		default:
		}
		lc, err := c.MakeLiveClient(ctx, liveID)
		if err != nil {
			// TODO Full or other error.
			log.Print(err)
			continue
		}
		ch, err := lc.StreamingComment(ctx, 0)
		if err != nil {
			log.Print(err)
			return 1
		}
		errCh := make(chan error)
		chatResultCh := make(chan *nico.ChatResult)
		go func() {
			var continueCnt int
			for {
				if err := lc.PostComment(ctx, comment, mail); err != nil {
					log.Print(err)
					errCh <- err
					return
				}
				cr := <-chatResultCh
				if *isPostOnce {
					errCh <- errors.New("post once")
					return
				}
				if cr.Status != 0 {
					continueCnt++
					if continueCnt > 1 {
						continueDuration += 10 * time.Second
					}
					time.Sleep(continueDuration)
				} else {
					continueCnt = 0
				}
			}
		}()
		for ci := range ch {
			var isBreak bool
			select {
			case err := <-errCh:
				log.Print(err)
				isBreak = true
			default:
			}
			if isBreak {
				if *isPostOnce {
					return 0
				}
				break
			}
			switch com := ci.(type) {
			case *nico.Thread:
				fmt.Printf("%#v\n", com)
			case *nico.ChatResult:
				chatResultCh <- com
				fmt.Printf("%#v\n", com)
			case *nico.Chat:
				if strings.Contains(com.Comment, hbIfseetnoComment) {
					continue
				}
				fmt.Println(com.Comment)
			}
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

func getSession(fp string) (string, error) {
	f, err := os.Open(fp)
	if err != nil {
		return "", err
	}
	defer f.Close()

	bs, err := ioutil.ReadAll(f)
	if err != nil {
		return "", err
	}

	return string(bs), nil
}

func saveSession(session, sessionFilePath string) error {
	if err := os.MkdirAll(filepath.Dir(sessionFilePath), 0700); err != nil {
		return err
	}

	if err := ioutil.WriteFile(sessionFilePath, []byte(session), 0600); err != nil {
		return err
	}

	return nil
}
