package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/rest"
	"strings"
	"time"
)

type SlackField struct {
	Title string `json:"title"`
	Value string `json:"value"`
	Short bool   `json:"short"`
}

type SlackAttachment struct {
	AuthorName string       `json:"author_name"`
	AuthorLink string       `json:"author_link"`
	Title      string       `json:"title"`
	TitleLink  string       `json:"title_link"`
	Text       string       `json:"text"`
	Fields     []SlackField `json:"fields"`
}

type SlackMessage struct {
	Attachments []SlackAttachment `json:"attachments"`
}

func resourceUrl(event *v1.Event) string {
	return os.Getenv("OPENSHIFT_CONSOLE_URL") + "/project/" + event.InvolvedObject.Namespace + "/browse/" + strings.ToLower(event.InvolvedObject.Kind) + "s/" + event.InvolvedObject.Name
}

func monitoringUrl(event *v1.Event) string {
	return os.Getenv("OPENSHIFT_CONSOLE_URL") + "project/" + event.InvolvedObject.Namespace + "/monitoring"
}

func notifySlack(event *v1.Event) {
	webhookUrl := os.Getenv("SLACK_WEBHOOK_URL")
	message := SlackMessage{
		Attachments: []SlackAttachment{
			{
				AuthorName: event.InvolvedObject.Namespace,
				AuthorLink: monitoringUrl(event),
				Title:      event.InvolvedObject.Name,
				TitleLink:  resourceUrl(event),
				Text:       event.Message,
				Fields: []SlackField{
					{
						Title: "Reason",
						Value: event.Reason,
						Short: true,
					},
					{
						Title: "Kind",
						Value: event.InvolvedObject.Kind,
						Short: true,
					},
				},
			},
		},
	}
	messageJson, err := json.Marshal(message)
	if err != nil {
		panic(err)
	}
	client := http.Client{}
	req, err := http.NewRequest("POST", webhookUrl, bytes.NewBufferString(string(messageJson)))
	req.Header.Set("Content-Type", "application/json")
	_, err = client.Do(req)
	if err != nil {
		fmt.Println("Unable to reach the server.")
	}
}

func watchEvents(clientset *kubernetes.Clientset, startTime time.Time) {
	watcher, err := clientset.CoreV1().Events("").Watch(v1.ListOptions{FieldSelector: "type=Warning"})
	if err != nil {
		panic(err.Error())
	}

	go func() {
		for watchEvent := range watcher.ResultChan() {
			event := watchEvent.Object.(*v1.Event)
			if event.FirstTimestamp.Time.After(startTime) {
				notifySlack(event)
			}
		}
	}()
}

func main() {
	startTime := time.Now()
	config, err := rest.InClusterConfig()
	if err != nil {
		panic(err.Error())
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	watchEvents(clientset, startTime)

	log.Println("Listening on port 8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
