package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/patrickmn/go-cache"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/rest"
	"strings"
	"time"
)

//create cache server
var (
	cachesvr = cache.New(1*time.Minute, 2*time.Minute)
)

type SlackField struct {
	Title string `json:"title"`
	Value string `json:"value"`
	Short bool   `json:"short"`
}

type SlackAttachment struct {
	Color      string       `json:"color"`
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
				Color:      "warning",
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

func watchEvents(clientset *kubernetes.Clientset) {
	startTime := time.Now()
	log.Printf("Watching events after %v", startTime)

	watcher, err := clientset.CoreV1().Events("").Watch(v1.ListOptions{FieldSelector: "type=Warning"})
	if err != nil {
		panic(err.Error())
	}

	for watchEvent := range watcher.ResultChan() {
		event := watchEvent.Object.(*v1.Event)
		if event.FirstTimestamp.Time.After(startTime) {
			log.Printf("Handling event: namespace: %v, container: %v, message: %v", event.InvolvedObject.Namespace, event.InvolvedObject.Name, event.Message)
			//determine message type first
			currentMessage, msgType := buildCachedEvent(event)
			// check if an identical event has already been sent
			cachedMessage, found := cachesvr.Get(msgType)
			if !found {
				log.Printf("Cache is empty, let's send the event to slack.")
				//cache is empty, let's proceed normally
				notifySlack(event)
				// cache event
				cachesvr.Set(msgType, currentMessage, 0)
				log.Printf("Cached event: %v. Event type: %v", currentMessage, msgType)
			} else {
				// do the cached events identical?
				log.Printf("Cache is not empty.")
				log.Printf("Cached event: %v", cachedMessage)
				log.Printf("Current event: %v. Event type: %v", currentMessage, msgType)

				if cachedMessage != currentMessage {
					// events are different, send to slack
					log.Printf("Events are different.")
					notifySlack(event)
					log.Printf("Event %v has been sent.", currentMessage)
					cachesvr.Set(msgType, currentMessage, 0)
					log.Printf("Event %v has been cached.", currentMessage)
				} else {
					log.Printf("Events are identical. Ignoring.")
				}
			}
		}
	}
}

func buildCachedEvent(event *v1.Event) (string, string) {
	// namespace_containernamefrompodname_message - special case for readiness and liveness messages
	var msgc []string

	//store namespace
	msgc = append(msgc, event.InvolvedObject.Namespace)

	// deduct container name
	s := strings.Split(event.InvolvedObject.Name, "-")
	//store container name
	msgc = append(msgc, s[0])

	//store message
	if strings.HasPrefix(event.Message, "Readiness") || strings.HasPrefix(event.Message, "Liveness") {
		// special case for Readiness/Liveness events
		// extract first part of message
		s := strings.Split(event.Message, ": Get http://10.")

		// build a more generic message
		r := strings.NewReplacer("Readiness", "Liveness/Readiness", "Liveness", "Liveness/Readiness", " ", "_")

		msgc = append(msgc, r.Replace(s[0]))

	} else {
		msgc = append(msgc, event.Message)
	}

	// construct value to be cached
	message := strings.Join(msgc, "_")

	// determine event type
	messageType := determineEventType(event.Message)

	return message, messageType
}

func determineEventType(msg string) string {
	eventType := "undefined"

	// get first message word
	s := strings.Fields(msg)
	firstWord := s[0]

	switch firstWord {
	case "Readiness":
		eventType = "Readiness_Liveness"
	case "Liveness":
		eventType = "Readiness_Liveness"
	case "No":
		eventType = "No_nodes_available"
	case "wanted":
		eventType = "wanted_to_free_memory"
	default:
		fmt.Println("cannot determine eventType.")
	}

	return eventType
}

func main() {
	config, err := rest.InClusterConfig()
	if err != nil {
		panic(err.Error())
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	go func() {
		for {
			watchEvents(clientset)
			time.Sleep(5 * time.Second)
		}
	}()

	log.Println("Listening on port 8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
