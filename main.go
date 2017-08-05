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
	"runtime"
	"strings"
	"time"

	"github.com/178inaba/nico"
	"github.com/howeyc/gopass"
	homedir "github.com/mitchellh/go-homedir"
)

const (
	defaultSessionFilePath = "lv-barrage/session"
	hbIfseetnoComment      = "/hb ifseetno "
)

var (
	isAnonymous  = flag.Bool("a", false, "Post anonymous user (184)")
	commentColor = flag.String("c", "", "Comment color")
	isPostOnce   = flag.Bool("o", false, "Post once")
)

func main() {
	log.SetFlags(0)
	log.SetPrefix(fmt.Sprintf("%s: ", os.Args[0]))
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: %s [-a] [-c <comment_color>] [-o] <live_id | live_url> <comment>\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()
	os.Exit(run())
}

func run() int {
	args := flag.Args()
	if len(args) != 2 {
		flag.Usage()
		return 1
	}
	liveID, err := nico.FindLiveID(args[0])
	if err != nil {
		log.Print(err)
		return 1
	}
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

	c, err := getClientWithSession(ctx, sessionFilePath)
	if err != nil {
		log.Print(err)
		return 1
	}

	if err := barrage(ctx, c, liveID, comment); err != nil {
		log.Print(err)
		return 1
	}

	return 0
}

func barrage(ctx context.Context, c *nico.Client, liveID, comment string) error {
	continueDuration := 10 * time.Second
	mail := getMail()
	for {
		select {
		case <-ctx.Done():
			// Signal interrupt.
			return nil
		default:
		}
		lc, err := c.MakeLiveClient(ctx, liveID)
		if err != nil {
			if pse, ok := err.(nico.PlayerStatusError); ok {
				switch pse.Code {
				case nico.PlayerStatusErrorCodeFull:
					fmt.Println("Continue: Seat is full")
					continue
				case nico.PlayerStatusErrorCodeRequireCommunityMember:
					if err := followCommunityFromLiveID(ctx, c, liveID); err != nil {
						return err
					}
					continue
				}
			}
			return err
		}
		ch, err := lc.StreamingComment(ctx, 0)
		if err != nil {
			return err
		}
		errCh := make(chan error)
		chatResultCh := make(chan *nico.ChatResult)
		go continuousCommentPost(ctx, lc, comment, mail, errCh, chatResultCh, continueDuration)
		if showComments(ch, errCh, chatResultCh) {
			return nil
		}
	}
}

func getMail() nico.Mail {
	mail := nico.Mail{CommentColor: *commentColor}
	if *isAnonymous {
		mail.Is184 = true
	}
	return mail
}

func followCommunityFromLiveID(ctx context.Context, c *nico.Client, liveID string) error {
	comID, err := c.GetCommunityIDFromLiveID(ctx, liveID)
	if err != nil {
		return err
	}
	if err := c.FollowCommunity(ctx, comID); err != nil {
		return err
	}
	return nil
}

func continuousCommentPost(ctx context.Context, lc *nico.LiveClient,
	comment string, mail nico.Mail, errCh chan error,
	chatResultCh chan *nico.ChatResult, continueDuration time.Duration) {
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
		time.Sleep(5 * time.Second)
	}
}

func showComments(ch chan nico.Comment, errCh chan error, chatResultCh chan *nico.ChatResult) bool {
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
				return true
			}
			return false
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
	return false
}

func getSessionFilePath() (string, error) {
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("APPDATA"), defaultSessionFilePath), nil
	}
	home, err := homedir.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", defaultSessionFilePath), nil
}

func getClientWithSession(ctx context.Context, sessionFilePath string) (*nico.Client, error) {
	c := nico.NewClient()
	userSession, err := getSession(sessionFilePath)
	if err != nil {
		mail, password, err := prompt(ctx)
		if err != nil {
			return nil, err
		}
		userSession, err = c.Login(ctx, mail, password)
		if err != nil {
			return nil, err
		}

		if err := saveSession(userSession, sessionFilePath); err != nil {
			return nil, err
		}
	} else {
		c.UserSession = userSession
	}
	return c, nil
}

func prompt(ctx context.Context) (string, string, error) {
	// Login mail address from stdin.
	fmt.Print("Mail: ")
	ch := make(chan string)
	go func() {
		var s string
		fmt.Scanln(&s)
		ch <- s
	}()
	var mail string
	select {
	case <-ctx.Done():
		return "", "", ctx.Err()
	case mail = <-ch:
	}

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
