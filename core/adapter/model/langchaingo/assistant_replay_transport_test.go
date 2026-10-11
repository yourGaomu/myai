package langchaingo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"myai/core/domain/generation"
	domainmessage "myai/core/domain/message"
	domainmodel "myai/core/domain/model"
	modelport "myai/core/port/model"
)

func TestAssistantReplayIsRequestScopedAndPreservesAuthenticationMode(t *testing.T) {
	for _, authType := range []domainmodel.AuthType{domainmodel.AuthTypeBearer, domainmodel.AuthTypeNone} {
		t.Run(string(authType), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Messages []struct {
						Role      string `json:"role"`
						Content   string `json:"content"`
						Reasoning string `json:"reasoning_content"`
					} `json:"messages"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					w.WriteHeader(400)
					return
				}
				if len(body.Messages) != 3 {
					t.Error("unexpected message count")
					w.WriteHeader(400)
					return
				}
				id := body.Messages[2].Content
				if body.Messages[1].Content != "answer-"+id || body.Messages[1].Reasoning != " reasoning-"+id+" " {
					t.Errorf("cross-request replay: %+v", body.Messages)
				}
				if authType == domainmodel.AuthTypeNone && r.Header.Get("Authorization") != "" {
					t.Error("unexpected authorization")
				}
				if authType == domainmodel.AuthTypeBearer && r.Header.Get("Authorization") != "Bearer local-key" {
					t.Error("authorization was lost")
				}
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"done\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
			}))
			defer server.Close()
			model, err := (OpenAIChatCompletionsAdapter{}).CreateModel(modelport.CreationConfig{
				BaseURL: server.URL + "/v1", APIKey: "local-key", ModelName: "test-model", AuthType: authType,
			})
			if err != nil {
				t.Fatal(err)
			}
			var group sync.WaitGroup
			for i := 0; i < 8; i++ {
				group.Add(1)
				go func(i int) {
					defer group.Done()
					id := fmt.Sprint(i)
					_, err := model.Generate(context.Background(), modelport.GenerateRequest{
						Messages: []domainmessage.Message{domainmessage.Text(domainmessage.RoleSystem, "system"),
							domainmessage.Assistant("answer-"+id, " reasoning-"+id+" ", nil), domainmessage.Text(domainmessage.RoleUser, id)},
						Settings: generation.SystemDefaults(),
					})
					if err != nil {
						t.Error(err)
					}
				}(i)
			}
			group.Wait()
		})
	}
}
