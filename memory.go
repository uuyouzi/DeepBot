package deepbot

import (
	"fmt"
	"log"
	"time"

	"github.com/cohesion-org/deepseek-go"
	"github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"
)

type msgType struct {
	UserID  uint64 `json:"user_id"`
	GroupID uint64 `json:"group_id"`
	SubType string `json:"sub_type"`
	Time    uint64 `json:"time"`
	Sender  struct {
		UserID   uint64 `json:"user_id"`
		Nickname string `json:"nickname"`
		Card     string `json:"card,omitempty"`
		Role     string `json:"role,omitempty"`
		Sex      string `json:"sex,omitempty"`
		Age      uint64 `json:"age,omitempty"`
	} `json:"sender"`
	Message []struct {
		Type string `json:"type"`
		Data struct {
			Text    string `json:"text,omitempty"`
			Name    string `json:"name,omitempty"`
			QQ      string `json:"qq"`
			ID      string `json:"id"`
			File    string `json:"file,omitempty"`
			Summary string `json:"summary,omitempty"`
			Data    string `json:"data,omitempty"`
			Content any    `json:"content,omitempty"`
		} `json:"data"`
	} `json:"message"`
	MessageID       uint64 `json:"message_id"`
	MessageSeq      uint64 `json:"message_seq"`
	MessageFormat   string `json:"message_format"`
	MessageType     string `json:"message_type"`
	MessageSentType string `json:"message_sent_type"`
	PostType        string `json:"post_type"`
	RealID          uint64 `json:"real_id"`
	SelfID          uint64 `json:"self_id"`
	RawMessage      string `json:"raw_message"`
	Font            uint64 `json:"font"`
}

type msgItem struct {
	MessageID uint64 `json:"message_id"`
	DateTime  string `json:"date_time"`
	UserName  string `json:"user_name"`
	UserID    uint64 `json:"user_id"`
	Content   string `json:"content"`
}

func (bot *DeepBot) buildSTM(ctx *zero.Ctx) {
	params := make(zero.Params)
	params["group_id"] = ctx.Event.GroupID
	params["message_seq"] = 0
	params["count"] = 300

	resp := ctx.CallAction("get_group_msg_history", params)
	if resp.Status != "ok" {
		return
	}

	var messages []*msgType
	raw := resp.Data.Get("messages").Raw
	err := jsonDecode([]byte(raw), &messages)
	if err != nil {
		log.Println("failed to read group history message:", err)
		return
	}

	// messages = messages[800:1100]

	var items []*msgItem
	for _, msg := range messages {
		var content string
		for _, m := range msg.Message {
			switch m.Type {
			case "text":
				content += fmt.Sprintf("[text]: %s\n", m.Data.Text)
			case "at":
				content += fmt.Sprintf("[at]: %s\n", m.Data.QQ)
			case "reply":
				content += fmt.Sprintf("[reply]: %s\n", m.Data.ID)
			}
		}
		if content == "" {
			continue
		}
		dateTime := time.Unix(int64(msg.Time), 0).Local().Format(time.DateTime)
		userName := msg.Sender.Nickname
		if msg.Sender.Card != "" {
			userName = msg.Sender.Card
		}

		item := &msgItem{
			MessageID: msg.MessageID,
			DateTime:  dateTime,
			UserName:  userName,
			UserID:    msg.Sender.UserID,
			Content:   content,
		}
		items = append(items, item)
	}

	output, err := jsonEncode(items)
	if err != nil {
		log.Println("failed to encode history message:", err)
		return
	}

	fmt.Println(string(output))

	// return

	// 	prompt := `
	// 以下是你加入的一个群聊中最近的消息(JSON格式)，你是一名活跃的群员，
	// 你现在要根据以下的历史对话内容来发送一条最符合你人设的消息。
	// 你不允许回复和最近几条消息相类似的内容，尽量有自己的看法。
	// 你的回复只需要包含你要的回复文本即可，不需要使用JSON格式。
	// 你不需要使用"[text]:"、"[at]:"、"[reply]:"来标识你的回复类型
	//
	// content中有三类消息:
	// [text]: 纯文本数据。
	// [at]: 此条消息@了某位其他群员，后面的参数是user_id。
	// [reply]: 此条消息回复了之前的一条消息，后面的参数是message_id。
	// ========================历史对话内容========================
	// `

	prompt := `
[人物设定]
你是一个记忆助理，可以从过往的聊天记录中总结出有价值的记忆。

[工作目标]
以下是你加入的一个群聊中最近的消息, 你需要从用户聊天记录中
提取出与群友们有价值的短期记忆和长期记忆。

[处理流程]
1. **信息分类**
   识别以下内容中的关键实体和事件，按类别归类：
   - 人物/宠物
   -  人物性格
   -  人物习惯
   - 兴趣爱好
   - 近期事件
   - 用户表达的情绪

2. **时间线梳理**
   按时间顺序排列重要事件，示例：
   - 2024-03-01：用户A提到养了一只猫，名字叫小白
   - 2024-03-15：用户A说周末带小白去公园

3. **生成总结**
     你总结的记忆格式为 日期, user_id, user_name, 总结出的记忆内容
   示例: "2025-03-04 19:11:22, 12345678, 用户A, 用户A喜欢吃苹果"
   每条记忆直接记得加换行。

[聊天记录结构]
示例:
  {
    "message_id": 132361297,
    "date_time": "2025-03-05 19:11:22",
    "user_name": "用户A",
    "user_id": 12345678,
    "content": "[reply]: 132361296\n[at]: 12345679\n[text]:  肯定的\n"
  },

  常规回复
  at人
  回复某人

message_id 是 消息的id
date_time 是该消息发送时的时间
user_name 是该用户在群聊中的昵称
user_id 是该用户的uid

content中有三类消息:
[text]: 纯文本数据。
[at]: 此条消息@了某位其他群员，后面的参数是user_id。
[reply]: 此条消息回复了之前的一条消息，后面的参数是message_id。

========================历史聊天记录=======================
`

	prompt = `
以下是你加入的一个群聊中最近的消息(JSON格式)，你是一名活跃的群员，
你现在要根据以下的历史对话内容来总结出与群友有价值的短期记忆和长期记忆。
记忆内容通常包含了确切的事件内容、个人爱好、个人习惯等。
你总结的记忆格式一条为 user_id + user_name + 总结出的记忆内容 + 换行

content中有三类消息:
[text]: 纯文本数据。
[at]: 此条消息@了某位其他群员，后面的参数是user_id。
[reply]: 此条消息回复了之前的一条消息，后面的参数是message_id。
========================历史对话内容=======================
`

	prompt += string(output)

	fmt.Println(len(prompt))

	req := &ChatRequest{
		Model:       deepseek.DeepSeekChat,
		Temperature: 0,
		TopP:        1,
		MaxTokens:   8192,
	}

	user := new(user)

	response, err := bot.seek(req, user, prompt)
	if err != nil {
		log.Println(err)
		return
	}
	fmt.Println(response.Answer)

	ctx.Send(message.Text(response.Answer))

	// for _, msg := range messages {
	// 	fmt.Println()
	//
	// 	fmt.Println(msg.Time, msg.Sender.Nickname, msg.Sender.UserID)
	// 	for _, m := range msg.Message {
	// 		switch m.Type {
	// 		case "text":
	// 			fmt.Println("text:", m.Data.Text)
	// 		case "at":
	// 			fmt.Println("at:", m.Data.QQ)
	// 		case "reply":
	// 			fmt.Println("reply:", m.Data.ID)
	// 		default:
	// 			fmt.Println(m.Type)
	// 		}
	// 	}
	// }
}
