package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/andfenastari/chatsim/core"
	"github.com/andfenastari/chatsim/shell/api"
)

var (
	chatsim = flag.String("chatsim", "http://localhost:8000/", "Chatsim server url. Default: 'https://localhost:8000/'")
	user    = flag.String("user", "", "User that is chatting.")
	peer    = flag.String("peer", "", "Peer to chat with.")
	port    = flag.String("port", "", "Port to listen for webhooks.")
)

type jsonObject = map[string]interface{}
type jsonArray = []interface{}

func main() {
	flag.Parse()

	go incoming()
	go outgoing()
}

func incoming() {
	client := &http.Client{}

	wreq := api.CreateWebhookRequest{
		URL: "http://localhost:" + *port,
	}
	rres, err := client.Post(fmt.Sprintf("%s/v17.0/%s/webhooks", *chatsim, *user), "text/json", jsonReader(wreq))
	if err != nil {
		panic(err)
	}

	var wres api.CreateWebhookResponse
	jsonDecode(rres.Body, wres)

	server := &http.ServeMux{}

	server.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		var req jsonObject
		err := json.NewDecoder(r.Body).Decode(&req)
		if err != nil {
			log.Fatalf("Couldn't decode webhook: %v", err)
		}

		entries := req["entry"].(jsonArray)
		entry := entries[0].(jsonObject)
		changes := entry["changes"].(jsonArray)
		change := changes[0].(jsonObject)
		value := change["value"].(jsonObject)
		messages := value["messages"].(jsonArray)
		message := messages[0].(core.Message)
		metadata := value["metadata"].(jsonObject)
		phone_number := metadata["phone_number_id"].(string)

		if phone_number != *peer {
			return
		}

		fmt.Printf("%s: %s\n", phone_number, message.Text.Body)
	})

	http.ListenAndServe(":9000", server)
	fmt.Print(wres.Id)
}

func outgoing() {
	reader := bufio.NewReader(os.Stdin)
	client := http.Client{}

	for {
		text, err := reader.ReadString('\n')

		if err != nil {
			panic(err)
		}

		msg := core.Message{
			To:   *peer,
			Type: "text",
			Text: &core.TextMessage{Body: text},
		}

		client.Post(fmt.Sprintf("%s/v17.0/%s/messages"), *chatsim, jsonReader(msg))
	}

}

func jsonReader(val any) io.Reader {
	b, err := json.Marshal(val)
	if err != nil {
		panic(err)
	}
	return bytes.NewReader(b)
}

func jsonDecode(r io.Reader, val any) {
	err := json.NewDecoder(r).Decode(val)
	if err != nil {
		panic(err)
	}
}
