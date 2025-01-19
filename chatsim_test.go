package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/davecgh/go-spew/spew"
)

type CreateMessageTest struct {
	Sender    string
	Message   *Message
	InitState *State
	EndState  *State
	Response  *CreateMessageResponse
}

func TestCreateMessage(t *testing.T) {
	tests := []CreateMessageTest{
		{
			Sender: "+00",
			Message: &Message{
				To:   "+11",
				Type: "text",
				Text: &TextMessage{
					Body: "Sup?",
				},
			},
			Response:  newCreateMessageResponse("+11"),
			InitState: &State{},
			EndState: &State{
				Chats: []*Chat{
					&Chat{
						User: "+11",
						Messages: []*Message{
							{
								To:   "+11",
								Type: "text",
								Text: &TextMessage{
									Body: "Sup?",
								},
							},
						},
					},
				},
			},
		},
		{
			Sender: "+11",
			Message: &Message{
				To:   "+00",
				Type: "text",
				Text: &TextMessage{
					Body: "Sup?",
				},
			},
			Response:  newCreateMessageResponse("+00"),
			InitState: &State{},
			EndState: &State{
				Chats: []*Chat{
					&Chat{
						User: "+11",
						Messages: []*Message{
							{
								To:   "+00",
								Type: "text",
								Text: &TextMessage{
									Body: "Sup?",
								},
							},
						},
					},
				},
			},
		},
		{
			Sender: "+00",
			Message: &Message{
				To:   "+22",
				Type: "text",
				Text: &TextMessage{
					Body: "Sup?",
				},
			},
			Response: newCreateMessageResponse("+22"),
			InitState: &State{
				Chats: []*Chat{
					{
						User: "+11",
					},
				},
			},
			EndState: &State{
				Chats: []*Chat{
					{
						User: "+11",
					},
					{
						User: "+22",
						Messages: []*Message{
							{
								To:   "+22",
								Type: "text",
								Text: &TextMessage{
									Body: "Sup?",
								},
							},
						},
					},
				},
			},
		},
	}

	for i, test := range tests {
		t.Run(fmt.Sprintf("Test %d", i), func(t *testing.T) {

			// Arrange
			server := newServer(test.InitState)
			w := httptest.NewRecorder()
			msg, err := json.Marshal(test.Message)
			body := bytes.NewReader(msg)
			req := httptest.NewRequest("POST", "/"+test.Sender+"/messages", body)

			// Act
			server.ServeHTTP(w, req)

			// Assert
			result := w.Result()
			if result.StatusCode != http.StatusOK {
				t.Fatalf("Response status mismatch. Expected 200, got %v", result.StatusCode)
			}

			var res *CreateMessageResponse
			err = json.NewDecoder(result.Body).Decode(&res)
			if err != nil {
				t.Errorf("Failed to decode body. %v", err)
			}

			if !reflect.DeepEqual(test.Response, res) {
				t.Errorf("Response mismatch. Expected %+v, got %+v", test.Response, res)
			}

			if !reflect.DeepEqual(test.EndState, server.State) {
				log.Print("OK")
				t.Errorf("State mismatch. Expected %s, got %s", spew.Sdump(test.EndState), spew.Sdump(server.State))
			}
		})
	}
}
