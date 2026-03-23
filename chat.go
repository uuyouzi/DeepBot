package deepbot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand/v2"
	"os"
	"strings"
	"time"

	"github.com/cohesion-org/deepseek-go"
	"github.com/wdvxdr1123/ZeroBot"
)

const maxToolCallLen = 128 * 1024

const promptToolCall = `
[外部函数调用指南]
   你可以使用浏览器来访问原先你访问不到的外部资源，具体请使用BrowseURL工具函数。
   你可以生成并且执行Go语言代码，来访问原先你访问不到的外部资源，具体请使用EvalGo工具函数。
   请注意如果你只是需要浏览网页，请优先使用BrowseURL，而不是生成相关代码使用EvalGo来访问。
   不要重复地访问同一个URL，以及不要递归访问网站内容中的出现URL。
   仅当你需要访问实时信息时才应该使用BrowseURL工具函数。
`

type chatResp struct {
	Answer    string
	Reasoning string
}

func (cr *chatResp) String() string {
	return cr.Answer
}

func (bot *DeepBot) onChat(ctx *zero.Ctx) {
	msg := ctx.MessageString()
	msg = strings.Replace(msg, "chat ", "", 1)
	fmt.Println("chat", ctx.Event.GroupID, msg)
	user := bot.getUser(ctx.Event.UserID)

	req := &ChatRequest{
		Model:       deepseek.DeepSeekChat,
		Temperature: 1.2,
		TopP:        1,
		MaxTokens:   8192,
	}
	resp, err := bot.chat(req, user, msg)
	if err != nil {
		log.Printf("failed to chat: %s\n", err)
		return
	}

	bot.reply(ctx, user, resp.Answer)
}

func (bot *DeepBot) onChatX(ctx *zero.Ctx) {
	msg := ctx.MessageString()
	msg = strings.Replace(msg, "chatx ", "", 1)
	fmt.Println("chatx", ctx.Event.GroupID, msg)
	user := bot.getUser(ctx.Event.UserID)

	req := &ChatRequest{
		Model:       deepseek.DeepSeekChat,
		Temperature: 1.2,
		TopP:        1,
		MaxTokens:   8192,
		Tools:       bot.tools,
	}
	resp, err := bot.chat(req, user, msg)
	if err != nil {
		log.Printf("failed to chatx: %s\n", err)
		return
	}

	bot.reply(ctx, user, resp.Answer)
}

func (bot *DeepBot) onReasoner(ctx *zero.Ctx) {
	msg := ctx.MessageString()
	msg = strings.Replace(msg, "ai ", "", 1)
	fmt.Println("ai", ctx.Event.GroupID, msg)
	user := bot.getUser(ctx.Event.UserID)

	req := &ChatRequest{
		Model:     deepseek.DeepSeekReasoner,
		MaxTokens: 8192,
	}
	resp, err := bot.chat(req, user, msg)
	if err != nil {
		log.Printf("failed to reasoner: %s\n", err)
		return
	}

	bot.reply(ctx, user, resp.Answer)
}

func (bot *DeepBot) onReasonerX(ctx *zero.Ctx) {
	msg := ctx.MessageString()
	msg = strings.Replace(msg, "aix ", "", 1)
	fmt.Println("aix", ctx.Event.GroupID, msg)
	user := bot.getUser(ctx.Event.UserID)

	req := &ChatRequest{
		Model:     deepseek.DeepSeekReasoner,
		MaxTokens: 8192,
	}
	resp, err := bot.chat(req, user, msg)
	if err != nil {
		log.Printf("failed to reasonerx: %s\n", err)
		return
	}

	tpl := `
<h3>思考过程</h3>
<div>%s</div>

<h3>回复内容</h3>
<div>%s</div>
`
	reasoning := resp.Reasoning
	if isMarkdown(reasoning) {
		reasoning = markdownToHTML(reasoning)
	}
	answer := resp.Answer
	if isMarkdown(answer) {
		answer = markdownToHTML(answer)
	}
	output := fmt.Sprintf(tpl, reasoning, answer)

	img, err := bot.htmlToImage(output)
	if err != nil {
		log.Println(err)
		return
	}
	sendImage(ctx, img)
}

func (bot *DeepBot) onCoder(ctx *zero.Ctx) {
	msg := ctx.MessageString()
	msg = strings.Replace(msg, "coder ", "", 1)
	fmt.Println("coder", ctx.Event.GroupID, msg)
	user := bot.getUser(ctx.Event.UserID)

	req := &ChatRequest{
		Model:       deepseek.DeepSeekChat,
		Temperature: 0,
		TopP:        1,
		MaxTokens:   8192,
	}
	resp, err := bot.chat(req, user, msg)
	if err != nil {
		log.Printf("failed to coder: %s\n", err)
		return
	}

	bot.reply(ctx, user, resp.Answer)
}

func (bot *DeepBot) onCoderX(ctx *zero.Ctx) {
	msg := ctx.MessageString()
	msg = strings.Replace(msg, "coderx ", "", 1)
	fmt.Println("coderx", ctx.Event.GroupID, msg)
	user := bot.getUser(ctx.Event.UserID)

	req := &ChatRequest{
		Model:       deepseek.DeepSeekChat,
		Temperature: 0,
		TopP:        1,
		MaxTokens:   8192,
		Tools:       bot.tools,
	}
	resp, err := bot.chat(req, user, msg)
	if err != nil {
		log.Printf("failed to coderx: %s\n", err)
		return
	}

	bot.reply(ctx, user, resp.Answer)
}

func (bot *DeepBot) onMessage(ctx *zero.Ctx) {
	if !ctx.Event.IsToMe || ctx.Event.GroupID != 0 {
		return
	}

	msg := ctx.MessageString()
	user := bot.getUser(ctx.Event.UserID)
	model := user.getModel()

	req := &ChatRequest{
		Model:     model,
		MaxTokens: 8192,
	}
	switch model {
	case deepseek.DeepSeekChat:
		req.Temperature = 1.2
		req.TopP = 1
		req.Tools = bot.tools
	case deepseek.DeepSeekReasoner:
	default:
		bot.sendText(ctx, "非法模型名称")
		return
	}
	if !user.canToolCall() {
		req.Tools = nil
		req.ToolChoice = nil
	}

	resp, err := bot.chat(req, user, msg)
	if err != nil {
		log.Printf("failed to on message: %s\n", err)
		return
	}

	bot.reply(ctx, user, resp.Answer)
}

func (bot *DeepBot) onGetModel(ctx *zero.Ctx) {
	user := bot.getUser(ctx.Event.UserID)
	model := user.getModel()

	bot.sendText(ctx, "当前模型: "+model)
}

func (bot *DeepBot) onSetModel(ctx *zero.Ctx) {
	args := textToArgN(ctx.MessageString(), 2)
	if len(args) != 2 {
		bot.sendText(ctx, "非法参数格式")
		return
	}

	model := args[1]
	switch model {
	case "r1":
		model = deepseek.DeepSeekReasoner
	case "chat":
		model = deepseek.DeepSeekChat
	case "8b":
		model = "deepseek-r1:8b" // 联合测试使用
	default:
		bot.sendText(ctx, "非法模型名称")
		return
	}

	user := bot.getUser(ctx.Event.UserID)
	user.setModel(model)

	bot.sendText(ctx, "设置模型成功")
}

func (bot *DeepBot) onEnableToolCall(ctx *zero.Ctx) {
	user := bot.getUser(ctx.Event.UserID)

	user.setToolCall(true)

	bot.sendText(ctx, "全局启用函数")
}

func (bot *DeepBot) onDisableToolCall(ctx *zero.Ctx) {
	user := bot.getUser(ctx.Event.UserID)

	user.setToolCall(false)

	bot.sendText(ctx, "全局禁用函数")
}

func (bot *DeepBot) onReset(ctx *zero.Ctx) {
	user := bot.getUser(ctx.Event.UserID)

	user.setRounds(nil)
	_ = os.Remove(fmt.Sprintf("data/conversation/%d/current.json", user.id))

	bot.sendText(ctx, "重置会话成功")
}

func (bot *DeepBot) onPoke(ctx *zero.Ctx) {
	if !ctx.Event.IsToMe {
		return
	}

	switch rand.IntN(8) {
	case 0:
		bot.sendText(ctx, "?")
	case 1:
		bot.sendText(ctx, "??")
	case 2:
		bot.sendText(ctx, "???")
	case 3:
		bot.sendText(ctx, "¿¿¿")
	case 4:
		bot.sendText(ctx, "别戳了")
	case 5:
		bot.sendText(ctx, "再戳我就要爆了")
	default:
		bot.replyEmoticon(ctx, nil)
	}
}

func (bot *DeepBot) chat(req *ChatRequest, user *user, msg string) (*chatResp, error) {
	var err error
	for i := 0; i < 3; i++ {
		var resp *chatResp
		resp, err = bot.tryChat(req, user, msg)
		if err == nil {
			bot.saveCurrentConversation(user)
			return resp, nil
		}
		var retry bool
		errStr := err.Error()
		for _, es := range []string{
			"failed to create chat completion",
			"receive empty message content",
		} {
			if strings.Contains(errStr, es) {
				retry = true
				break
			}
		}
		if retry {
			fmt.Printf("[warning] retry send chat request with %d times\n", i+1)
			time.Sleep(3 * time.Second)
			continue
		}
		break
	}
	return nil, err
}

func (bot *DeepBot) saveCurrentConversation(user *user) {
	rounds := user.getRounds()
	output, err := jsonEncode(&rounds)
	if err != nil {
		log.Println("failed to encode current conversation:", err)
		return
	}
	path := fmt.Sprintf("data/conversation/%d/current.json", user.id)
	err = os.WriteFile(path, output, 0600)
	if err != nil {
		log.Println("failed to save current conversation:", err)
		return
	}
}

func (bot *DeepBot) tryChat(req *ChatRequest, user *user, msg string) (*chatResp, error) {
	var messages []ChatMessage
	// build and append system prompt
	character := user.getCharacter()
	// if len(req.Tools) > 0 && req.Model != deepseek.DeepSeekReasoner {
	// 	character += "\n\n" + promptToolCall
	// }
	if character != "" {
		messages = append(messages, ChatMessage{
			Role:    deepseek.ChatMessageRoleSystem,
			Content: character,
		})
	}
	// append user past round message
	rounds := user.getRounds()
	for i := 0; i < len(rounds); i++ {
		question := rounds[i].Question
		if question.Role != "" {
			messages = append(messages, question)
		}
		answer := rounds[i].Answer
		if answer.Role != "" {
			messages = append(messages, answer)
		}
	}

	// fmt.Println("================================================")
	// fmt.Println(messages)
	// fmt.Println("================================================")

	// append user question
	question := ChatMessage{
		Role:    deepseek.ChatMessageRoleUser,
		Content: msg,
	}
	messages = append(messages, question)
	// send request
	req.Messages = messages
	resp, err := bot.client.CreateChatCompletion(context.Background(), req)
	if err != nil {
		return nil, fmt.Errorf("failed to create chat completion: %s", err)
	}
	// reset usage counter before process tool calls
	resetToolLimit(user)
	resp, err = bot.doToolCalls(req, resp, user)
	if err != nil {
		return nil, fmt.Errorf("failed to process tool call: %s", err)
	}
	// process response
	cm := resp.Choices[0].Message
	if cm.Role != deepseek.ChatMessageRoleAssistant {
		return nil, errors.New("invalid message role: " + cm.Role)
	}
	content := cm.Content
	if content == "" {
		return nil, errors.New("receive empty message content")
	}
	reasoning := cm.ReasoningContent
	answer := ChatMessage{
		Role:    deepseek.ChatMessageRoleAssistant,
		Content: content,
	}
	rounds = append(rounds, &round{
		Question: question,
		Answer:   answer,
	})
	user.setRounds(rounds)

	fmt.Println("==================chat response=================")
	fmt.Println(content)
	if reasoning != "" {
		fmt.Println("----------------reasoning content----------------")
		fmt.Println(reasoning)
	}
	fmt.Println("------------------------------------------------")
	usage := resp.Usage
	fmt.Println("prompt token:", usage.PromptTokens, "completion token:", usage.CompletionTokens)
	fmt.Println("cache hit:", usage.PromptCacheHitTokens, "cache miss:", usage.PromptCacheMissTokens)
	fmt.Println("================================================")

	cr := &chatResp{
		Answer:    content,
		Reasoning: reasoning,
	}
	return cr, nil
}

func (bot *DeepBot) doToolCalls(req *ChatRequest, resp *ChatResponse, user *user) (*ChatResponse, error) {
	msg := resp.Choices[0].Message
	toolCalls := msg.ToolCalls
	numCalls := len(toolCalls)
	if numCalls == 0 {
		return resp, nil
	}
	fmt.Println("num tool calls:", numCalls)

	question := ChatMessage{
		Role:      deepseek.ChatMessageRoleAssistant,
		Content:   msg.Content,
		ToolCalls: toolCalls,
	}
	var answers []ChatMessage
	for i := 0; i < numCalls; i++ {
		toolCall := toolCalls[i]
		answer, err := bot.doToolCall(toolCall, user)
		if err != nil {
			return nil, err
		}
		if len(answer) > maxToolCallLen {
			answer = answer[:maxToolCallLen]
		}
		answers = append(answers, ChatMessage{
			Role:       deepseek.ChatMessageRoleTool,
			Content:    answer,
			ToolCallID: toolCall.ID,
		})
		fmt.Println(answer)
	}

	messages := req.Messages
	messages = append(messages, question)
	messages = append(messages, answers...)
	toolReq := &ChatRequest{
		Model:       req.Model,
		Messages:    messages,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		MaxTokens:   8192,
		Tools:       updateTools(user, req.Tools),
	}
	resp, err := bot.client.CreateChatCompletion(context.Background(), toolReq)
	if err != nil {
		return nil, err
	}

	// 2025/02/22 经过测试，模型暂时不会将工具函数的返回结果应用在全局上下文，只有当前一轮的问答。
	// 可能是为了避免不及时的函数结果，但是后期也许可以加一个标志，使其可以应用在全局上下文。
	// rounds := user.getRounds()
	// rounds = append(rounds, &round{
	// 	Question: question,
	// })
	// for i := 0; i < len(answers); i++ {
	// 	rounds = append(rounds, &round{
	// 		Answer: answer[i],
	// 	})
	// }
	// user.setRounds(rounds)
	return bot.doToolCalls(toolReq, resp, user)
}

func (bot *DeepBot) doToolCall(toolCall deepseek.ToolCall, user *user) (string, error) {
	arguments := toolCall.Function.Arguments
	decoder := json.NewDecoder(strings.NewReader(arguments))
	decoder.DisallowUnknownFields()
	var (
		answer string
		err    error
	)
	fnName := toolCall.Function.Name
	switch fnName {
	case fnGetTime:
		answer, err = bot.onGetTime(user)
	case fnSearchWeb:
		answer, err = bot.onSearchWeb(decoder, user)
	case fnSearchImage:
		answer, err = bot.onSearchImage(decoder, user)
	case fnBrowseURL:
		answer, err = bot.onBrowseURL(decoder, user)
	case fnEvalGo:
		answer, err = bot.onEvalGo(decoder, user)
	default:
		return "", fmt.Errorf("unknown function: %s", fnName)
	}
	return answer, err
}

func (bot *DeepBot) onGetTime(user *user) (string, error) {
	err := checkToolLimit(user, fnGetTime)
	if err != nil {
		return "", err
	}
	return onGetTime(), nil
}

func (bot *DeepBot) onSearchWeb(decoder *json.Decoder, user *user) (string, error) {
	err := checkToolLimit(user, fnSearchWeb)
	if err != nil {
		return "", err
	}

	args := struct {
		Keyword string `json:"keyword"`
	}{}
	err = decoder.Decode(&args)
	if err != nil {
		return "", err
	}

	config := bot.config.SearchAPI
	timeout := time.Duration(config.Timeout) * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cfg := &searchCfg{
		EngineID: config.EngineID,
		APIKey:   config.APIKey,
		ProxyURL: config.ProxyURL,
	}
	output, err := onSearchWeb(ctx, cfg, args.Keyword)
	if err != nil {
		return "failed to search web: " + err.Error(), nil
	}
	return output, nil
}

func (bot *DeepBot) onSearchImage(decoder *json.Decoder, user *user) (string, error) {
	err := checkToolLimit(user, fnSearchImage)
	if err != nil {
		return "", err
	}

	args := struct {
		Keyword string `json:"keyword"`
		Size    string `json:"size"`
	}{}
	err = decoder.Decode(&args)
	if err != nil {
		return "", err
	}

	config := bot.config.SearchAPI
	timeout := time.Duration(config.Timeout) * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cfg := &searchCfg{
		EngineID: config.EngineID,
		APIKey:   config.APIKey,
		ProxyURL: config.ProxyURL,
	}
	output, err := onSearchImage(ctx, cfg, args.Keyword, args.Size)
	if err != nil {
		return "failed to search image: " + err.Error(), nil
	}
	return output, nil
}

func (bot *DeepBot) onBrowseURL(decoder *json.Decoder, user *user) (string, error) {
	err := checkToolLimit(user, fnBrowseURL)
	if err != nil {
		return "", err
	}

	args := struct {
		URL string `json:"url"`
	}{}
	err = decoder.Decode(&args)
	if err != nil {
		return "", err
	}

	timeout := time.Duration(bot.config.Browser.Timeout) * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	options := bot.getChromedpOptions()
	output, err := onBrowseURL(ctx, options, args.URL)
	if err != nil {
		return "Chromedp Error: " + err.Error(), nil
	}
	return output, nil
}

func (bot *DeepBot) onEvalGo(decoder *json.Decoder, user *user) (string, error) {
	err := checkToolLimit(user, fnEvalGo)
	if err != nil {
		return "", err
	}

	args := struct {
		Src string `json:"src"`
	}{}
	err = decoder.Decode(&args)
	if err != nil {
		return "", err
	}

	timeout := time.Duration(bot.config.EvalGo.Timeout) * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	output, err := onEvalGo(ctx, args.Src)
	if err != nil {
		return "Go Error: " + err.Error(), nil
	}
	return output, nil
}

func resetToolLimit(user *user) {
	for _, tool := range toolList {
		user.setContext("Usage_"+tool.Name, 0)
	}
}

func checkToolLimit(user *user, name string) error {
	tool := toolList[name]
	key := "Usage_" + tool.Name
	usage := user.getContext(key).(int)
	if usage >= tool.Limit {
		return fmt.Errorf("too many calls about %s", tool.Name)
	}
	user.setContext(key, usage+1)
	return nil
}

func reachToolLimit(user *user, name string) bool {
	tool := toolList[name]
	key := "Usage_" + tool.Name
	usage := user.getContext(key).(int)
	if usage >= tool.Limit {
		return true
	}
	return false
}

func updateTools(user *user, tools []deepseek.Tool) []deepseek.Tool {
	var result []deepseek.Tool
	for _, tool := range tools {
		if !reachToolLimit(user, tool.Function.Name) {
			result = append(result, tool)
		}
	}
	return result
}

// case "GetLocation":
// 	answer = "当前城市是: 汉堡王"
// case "GetTemperature":
// 	answer = "当前温度是: 8℃"
// case "GetRelativeHumidity":
// 	answer = "当前相对湿度是: 32%"

// func chatStream(client *deepseek.Client, request *deepseek.StreamChatCompletionRequest) (string, error) {
// 	stream, err := client.CreateChatCompletionStream(context.Background(), request)
// 	if err != nil {
// 		return "", fmt.Errorf("failed to create chat completion stream: %s", err)
// 	}
// 	defer func() { _ = stream.Close() }()
// 	var response string
// 	for {
// 		var resp *deepseek.StreamChatCompletionResponse
// 		resp, err = stream.Recv()
// 		if err == io.EOF {
// 			err = nil
// 			break
// 		}
// 		if err != nil {
// 			err = fmt.Errorf("failed to receive chat completion response: %s", err)
// 			break
// 		}
// 		for _, choice := range resp.Choices {
// 			response += choice.Delta.Content

// 			fmt.Print(choice.Delta.Content)
// 		}
// 	}
// 	if response == "" {
// 		return "", errors.New("receive empty response")
// 	}
// 	return response, err
//
