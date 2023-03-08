package main

import (
	"context"
	"log"
	"math/rand"
	"path/filepath"
	"strconv"
	"strings"

	gogpt "github.com/sashabaranov/go-gpt3"
	"gopkg.in/irc.v3"
)

func completion(m *irc.Message, message string, c *irc.Client, aiClient *gogpt.Client, ctx context.Context, model string, cost float64) {
	var responseString string

	req := gogpt.CompletionRequest{
		Model:       model,
		MaxTokens:   config.OpenAI.Tokens,
		Prompt:      message,
		Temperature: config.OpenAI.Temperature,
	}

	if model == gogpt.CodexCodeDavinci002 {
		req = gogpt.CompletionRequest{
			Model:            model,
			MaxTokens:        config.OpenAI.Tokens,
			Prompt:           message,
			Temperature:      0,
			TopP:             1,
			FrequencyPenalty: 0,
			PresencePenalty:  0,
		}
	}

	// Process a completion request
	c.WriteMessage(&irc.Message{
		Command: "PRIVMSG",
		Params: []string{
			m.Params[0],
			"Processing: " + message,
		},
	})

	// Perform the actual API request to openAI
	resp, err := aiClient.CreateCompletion(ctx, req)
	if err != nil {
		c.WriteMessage(&irc.Message{
			Command: "PRIVMSG",
			Params: []string{
				m.Params[0],
				err.Error(),
			},
		})

		// err.Error() contains You exceeded your current quota
		if strings.Contains(err.Error(), "You exceeded your current quota") {
			log.Println("Key " + whatKey + " has exceeded its quota")
		}

		return
	}

	// resp.Usage.TotalTokens / 1000 * cost
	total := strconv.FormatFloat((float64(resp.Usage.TotalTokens)/1000)*cost, 'f', 5, 64)

	responseString = strings.TrimSpace(resp.Choices[0].Text) + " ($" + total + ")"

	chunkToIrc(c, m.Params[0], responseString)
}

// Annoying reply to chats
func replyToChats(m *irc.Message, message string, c *irc.Client, aiClient *gogpt.Client, ctx context.Context) {
	req := gogpt.CompletionRequest{
		Model:       gogpt.GPT3TextDavinci003,
		MaxTokens:   config.OpenAI.Tokens,
		Prompt:      "As an " + config.AiBird.ChatPersonality + " reply to the following irc chats: " + message + ".",
		Temperature: config.OpenAI.Temperature,
	}

	// Perform the actual API request to openAI
	resp, err := aiClient.CreateCompletion(ctx, req)
	if err != nil {
		c.WriteMessage(&irc.Message{
			Command: "PRIVMSG",
			Params: []string{
				m.Params[0],
				err.Error(),
			},
		})

		// err.Error() contains You exceeded your current quota
		if strings.Contains(err.Error(), "You exceeded your current quota") {
			log.Println("Key " + whatKey + " has exceeded its quota")
		}

		return
	}

	chunkToIrc(c, m.Params[0], strings.TrimSpace(resp.Choices[0].Text))
}

func chatGpt(name string, m *irc.Message, message []gogpt.ChatCompletionMessage, c *irc.Client, aiClient *gogpt.Client, ctx context.Context) {
	req := gogpt.ChatCompletionRequest{
		Model:       gogpt.GPT3Dot5Turbo,
		MaxTokens:   config.OpenAI.Tokens,
		Messages:    message,
		Temperature: config.OpenAI.Temperature,
	}

	// Perform the actual API request to openAI
	resp, err := aiClient.CreateChatCompletion(ctx, req)
	if err != nil {
		c.WriteMessage(&irc.Message{
			Command: "PRIVMSG",
			Params: []string{
				m.Params[0],
				err.Error(),
			},
		})

		// err.Error() contains You exceeded your current quota
		if strings.Contains(err.Error(), "You exceeded your current quota") {
			log.Println("Key " + whatKey + " has exceeded its quota")
		}

		return
	}

	// for each ChatCompletionChoice
	for _, choice := range resp.Choices {
		// for each ChatCompletionMessage
		chunkToIrc(c, m.Prefix.Name, strings.TrimSpace(choice.Message.Content))

		key := []byte(name + "_" + m.Params[0] + "_chats_cache_gpt_" + m.Prefix.Name)
		message := "ASSISTANT: " + strings.TrimSpace(choice.Message.Content)
		chatList, err := birdBase.Get(key)
		if err != nil {
			log.Println(err)
			return
		}

		// reverse sliceChatList, seriously golang?
		for i := len(chatList)/2 - 1; i >= 0; i-- {
			opp := len(chatList) - 1 - i
			chatList[i], chatList[opp] = chatList[opp], chatList[i]
		}

		if len(chatList) >= config.AiBird.ChatGptTotalMessages {
			chatList = chatList[:config.AiBird.ChatGptTotalMessages]
		}

		birdBase.Put(key, []byte(message+"."+"\n"+string(chatList)))
	}
}

func dalle(m *irc.Message, message string, c *irc.Client, aiClient *gogpt.Client, ctx context.Context, size string) {
	req := gogpt.ImageRequest{
		Prompt: message,
		Size:   size,
		N:      1,
	}

	// Alert the irc chan that the bot is processing
	c.WriteMessage(&irc.Message{
		Command: "PRIVMSG",
		Params: []string{
			m.Params[0],
			"Processing Dall-E: " + message,
		},
	})

	resp, err := aiClient.CreateImage(ctx, req)
	if err != nil {
		c.WriteMessage(&irc.Message{
			Command: "PRIVMSG",
			Params: []string{
				m.Params[0],
				err.Error(),
			},
		})
		return
	}

	daleResponse := saveDalleRequest(message, resp.Data[0].URL)

	c.WriteMessage(&irc.Message{
		Command: "PRIVMSG",
		Params: []string{
			m.Params[0],
			m.Prefix.Name + ": " + daleResponse,
		},
	})
}

func saveDalleRequest(prompt string, url string) string {
	// Clean the filename, there has to be a better way to do this
	slug := cleanFileName(prompt)

	randValue := rand.Int63n(10000)
	// Place a random number on the end to (maybe almost) avoid overwriting duplicate requests
	fileName := slug + "_" + strconv.FormatInt(randValue, 4) + ".png"

	downloadFile(url, fileName)

	// append the current pwd to fileName
	fileName = filepath.Base(fileName)

	// download image
	content := fileHole("https://filehole.org/", fileName)

	return string(content)
}
