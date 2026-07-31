package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/gorilla/websocket"
)

const usage = `Commands:
  join            enter the matchmaking queue
  leave           leave the queue
  move <0-8>      play a cell
  state           reprint the board
  resign          give up the current match
  quit            close the client
`

type credentials struct {
	Name     string `json:"name"`
	Password string `json:"password"`
}

type tokenResponse struct {
	Token string `json:"token"`
}

type errorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type matchState struct {
	MatchID  string   `json:"match_id"`
	Cells    []string `json:"cells"`
	YourMark string   `json:"your_mark"`
	Turn     string   `json:"turn"`
	Status   string   `json:"status"`
	Outcome  string   `json:"outcome"`
	Opponent string   `json:"opponent"`
	Finished bool     `json:"finished"`
}

type frame struct {
	Type    string      `json:"type"`
	Of      string      `json:"of"`
	MatchID string      `json:"match_id"`
	State   *matchState `json:"state"`
	Code    string      `json:"code"`
	Message string      `json:"message"`
}

func main() {
	var (
		addr     = flag.String("addr", "localhost:8080", "server host:port")
		name     = flag.String("name", "", "player name")
		password = flag.String("password", "", "player password")
		register = flag.Bool("register", false, "register instead of logging in")
	)
	flag.Parse()

	if *name == "" || *password == "" {
		fmt.Fprintln(os.Stderr, "both -name and -password are required")
		os.Exit(1)
	}

	if err := run(*addr, *name, *password, *register); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(addr, name, password string, register bool) error {
	token, err := authenticate(addr, name, password, register)
	if err != nil {
		return err
	}

	wsURL := url.URL{Scheme: "ws", Host: addr, Path: "/ws"}
	header := http.Header{"Authorization": []string{"Bearer " + token}}

	conn, _, err := websocket.DefaultDialer.Dial(wsURL.String(), header)
	if err != nil {
		return fmt.Errorf("connecting to %s: %w", wsURL.String(), err)
	}
	defer conn.Close()

	fmt.Printf("Connected as %s.\n\n%s\n", name, usage)

	// Server frames arrive unprompted, so they are printed from their own
	// goroutine rather than in reply to a command.
	go readFrames(conn)

	return readCommands(conn)
}

func authenticate(addr, name, password string, register bool) (string, error) {
	path := "/login"
	if register {
		path = "/register"
	}

	body, err := json.Marshal(credentials{Name: name, Password: password})
	if err != nil {
		return "", err
	}

	resp, err := http.Post("http://"+addr+path, "application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		var e errorResponse
		json.NewDecoder(resp.Body).Decode(&e)

		return "", fmt.Errorf("%s: %s", e.Code, e.Message)
	}

	var tok tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return "", err
	}

	return tok.Token, nil
}

func readFrames(conn *websocket.Conn) {
	for {
		var f frame
		if err := conn.ReadJSON(&f); err != nil {
			fmt.Println("\nConnection closed.")
			os.Exit(0)
		}

		switch f.Type {
		case "error":
			fmt.Printf("\n! %s (%s)\n> ", f.Message, f.Code)

		case "ack":
			fmt.Printf("\nok: %s\n> ", f.Of)

		case "opponent_left":
			fmt.Println("\nYour opponent left the match.")
			printState(f.Type, f.State)

		default:
			printState(f.Type, f.State)
		}
	}
}

func readCommands(conn *websocket.Conn) error {
	scanner := bufio.NewScanner(os.Stdin)

	fmt.Print("> ")
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 0 {
			fmt.Print("> ")
			continue
		}

		if fields[0] == "quit" {
			return nil
		}

		cmd := map[string]any{"type": fields[0]}
		if fields[0] == "move" {
			if len(fields) < 2 {
				fmt.Print("move needs a cell number\n> ")
				continue
			}

			cell, err := strconv.Atoi(fields[1])
			if err != nil {
				fmt.Print("cell must be a number from 0 to 8\n> ")
				continue
			}

			cmd["cell"] = cell
		}

		if err := conn.WriteJSON(cmd); err != nil {
			return err
		}
	}

	return scanner.Err()
}

func printState(event string, s *matchState) {
	if s == nil {
		return
	}

	fmt.Printf("\n[%s] you are %s against %s\n\n", event, s.YourMark, s.Opponent)

	for row := range 3 {
		cells := make([]string, 0, 3)

		for col := range 3 {
			c := s.Cells[row*3+col]
			if c == "" {
				c = strconv.Itoa(row*3 + col)
			}

			cells = append(cells, c)
		}

		fmt.Printf("  %s\n", strings.Join(cells, " | "))
		if row < 2 {
			fmt.Println("  ---------")
		}
	}

	if s.Finished {
		fmt.Printf("\nMatch over: %s (%s)\n> ", s.Status, s.Outcome)
		return
	}

	fmt.Printf("\nTurn: %s\n> ", s.Turn)
}
