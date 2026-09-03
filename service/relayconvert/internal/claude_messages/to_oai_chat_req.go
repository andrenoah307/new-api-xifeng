package claudemessages

import (
	"fmt"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relaymeta "github.com/QuantumNous/new-api/service/relayconvert/internal/meta"
)

const (
	webSearchMaxUsesLow              = 1
	webSearchMaxUsesMedium           = 5
	webSearchMaxUsesHigh             = 10
	claudeToolResultRelocateMediaEnv = "CLAUDE_TOOL_RESULT_RELOCATE_MEDIA"
	toolResultMediaRelocatedMarker   = "[tool_result_image_relocated]"
	toolResultMediaFallbackMarker    = "[tool_result_media_fallback]"
)

type openRouterRequestReasoning struct {
	Enabled   bool   `json:"enabled"`
	Effort    string `json:"effort,omitempty"`
	MaxTokens int    `json:"max_tokens,omitempty"`
	Exclude   bool   `json:"exclude,omitempty"`
}

func ClaudeMessagesRequestToOpenAIChat(claudeRequest dto.ClaudeRequest, info *relaycommon.RelayInfo) (*dto.GeneralOpenAIRequest, error) {
	openAIRequest := dto.GeneralOpenAIRequest{
		Model:       claudeRequest.Model,
		Temperature: claudeRequest.Temperature,
	}
	relocateToolResultMedia := common.GetEnvOrDefaultBool(claudeToolResultRelocateMediaEnv, true)
	if info != nil {
		info.ToolResultImageCount = 0
		info.ToolResultImageBase64Chars = 0
		info.ToolResultMediaTypes = nil
		info.ToolResultMediaFallback = false
	}
	if claudeRequest.MaxTokens != nil {
		openAIRequest.MaxTokens = common.GetPointer(*claudeRequest.MaxTokens)
	}
	if claudeRequest.TopP != nil {
		openAIRequest.TopP = common.GetPointer(*claudeRequest.TopP)
	}
	if claudeRequest.TopK != nil {
		openAIRequest.TopK = common.GetPointer(*claudeRequest.TopK)
	}
	if claudeRequest.Stream != nil {
		openAIRequest.Stream = common.GetPointer(*claudeRequest.Stream)
	}

	isOpenRouter := relaymeta.RelayInfoChannelType(info) == constant.ChannelTypeOpenRouter
	if isOpenRouter {
		if effort := claudeRequest.GetEfforts(); effort != "" {
			effortBytes, _ := common.Marshal(effort)
			openAIRequest.Verbosity = effortBytes
		}
		if claudeRequest.Thinking != nil {
			var reasoningConfig openRouterRequestReasoning
			if claudeRequest.Thinking.Type == "enabled" {
				reasoningConfig = openRouterRequestReasoning{
					Enabled:   true,
					MaxTokens: claudeRequest.Thinking.GetBudgetTokens(),
				}
			} else if claudeRequest.Thinking.Type == "adaptive" {
				reasoningConfig = openRouterRequestReasoning{
					Enabled: true,
				}
			}
			reasoningJSON, err := common.Marshal(reasoningConfig)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal reasoning: %w", err)
			}
			openAIRequest.Reasoning = reasoningJSON
		}
	} else if info != nil {
		thinkingSuffix := "-thinking"
		if strings.HasSuffix(info.OriginModelName, thinkingSuffix) &&
			!strings.HasSuffix(openAIRequest.Model, thinkingSuffix) {
			openAIRequest.Model = openAIRequest.Model + thinkingSuffix
		}
	}

	if len(claudeRequest.StopSequences) == 1 {
		openAIRequest.Stop = claudeRequest.StopSequences[0]
	} else if len(claudeRequest.StopSequences) > 1 {
		openAIRequest.Stop = claudeRequest.StopSequences
	}

	tools, _ := common.Any2Type[[]dto.Tool](claudeRequest.Tools)
	openAITools := make([]dto.ToolCallRequest, 0)
	for _, claudeTool := range tools {
		openAITool := dto.ToolCallRequest{
			Type: "function",
			Function: dto.FunctionRequest{
				Name:        claudeTool.Name,
				Description: claudeTool.Description,
				Parameters:  claudeTool.InputSchema,
			},
		}
		openAITools = append(openAITools, openAITool)
	}
	openAIRequest.Tools = openAITools

	openAIMessages := make([]dto.Message, 0)
	if claudeRequest.System != nil {
		if claudeRequest.IsStringSystem() && claudeRequest.GetStringSystem() != "" {
			openAIMessage := dto.Message{
				Role: "system",
			}
			openAIMessage.SetStringContent(claudeRequest.GetStringSystem())
			openAIMessages = append(openAIMessages, openAIMessage)
		} else {
			systems := claudeRequest.ParseSystem()
			if len(systems) > 0 {
				openAIMessage := dto.Message{
					Role: "system",
				}
				isOpenRouterClaude := isOpenRouter && strings.HasPrefix(relaymeta.RelayInfoUpstreamModelName(info), "anthropic/claude")
				if isOpenRouterClaude {
					systemMediaMessages := make([]dto.MediaContent, 0, len(systems))
					for _, system := range systems {
						message := dto.MediaContent{
							Type:         "text",
							Text:         system.GetText(),
							CacheControl: system.CacheControl,
						}
						systemMediaMessages = append(systemMediaMessages, message)
					}
					openAIMessage.SetMediaContent(systemMediaMessages)
				} else {
					systemStr := ""
					for _, system := range systems {
						if system.Text != nil {
							systemStr += *system.Text
						}
					}
					openAIMessage.SetStringContent(systemStr)
				}
				openAIMessages = append(openAIMessages, openAIMessage)
			}
		}
	}

	var toolCallNames map[string]string
	toolCallNamesLoaded := false
	for _, claudeMessage := range claudeRequest.Messages {
		openAIMessage := dto.Message{
			Role: claudeMessage.Role,
		}
		if claudeMessage.IsStringContent() {
			openAIMessage.SetStringContent(claudeMessage.GetStringContent())
		} else {
			content, err := claudeMessage.ParseContent()
			if err != nil {
				return nil, err
			}
			var toolCalls []dto.ToolCallRequest
			mediaMessages := make([]dto.MediaContent, 0, len(content))

			for _, mediaMsg := range content {
				switch mediaMsg.Type {
				case "text", "input_text":
					message := dto.MediaContent{
						Type:         "text",
						Text:         mediaMsg.GetText(),
						CacheControl: mediaMsg.CacheControl,
					}
					mediaMessages = append(mediaMessages, message)
				case "image":
					imageData := fmt.Sprintf("data:%s;base64,%s", mediaMsg.Source.MediaType, mediaMsg.Source.Data)
					mediaMessage := dto.MediaContent{
						Type:     "image_url",
						ImageUrl: &dto.MessageImageUrl{Url: imageData},
					}
					mediaMessages = append(mediaMessages, mediaMessage)
				case "tool_use":
					toolCall := dto.ToolCallRequest{
						ID:   mediaMsg.Id,
						Type: "function",
						Function: dto.FunctionRequest{
							Name:      mediaMsg.Name,
							Arguments: requestToJSONString(mediaMsg.Input),
						},
					}
					toolCalls = append(toolCalls, toolCall)
				case "tool_result":
					toolName := mediaMsg.Name
					if toolName == "" {
						if !toolCallNamesLoaded {
							toolCallNames = claudeRequest.ToolCallNameIndex()
							toolCallNamesLoaded = true
						}
						toolName = toolCallNames[mediaMsg.ToolUseId]
					}
					oaiToolMessage := dto.Message{
						Role:       "tool",
						Name:       &toolName,
						ToolCallId: mediaMsg.ToolUseId,
					}
					if mediaMsg.IsStringContent() {
						oaiToolMessage.SetStringContent(mediaMsg.GetStringContent())
					} else {
						mediaContents := mediaMsg.ParseMediaContent()
						hasNonTextContent := false
						for _, content := range mediaContents {
							if content.Type != "text" && content.Type != "input_text" {
								hasNonTextContent = true
								break
							}
						}
						if !hasNonTextContent {
							// Keep the legacy byte-for-byte representation for string-like
							// and unparseable tool_result content.
							encodedJSON, _ := common.Marshal(mediaContents)
							oaiToolMessage.SetStringContent(string(encodedJSON))
						} else {
							var toolText strings.Builder
							mediaMoved := false
							mediaFallback := false
							for _, content := range mediaContents {
								switch content.Type {
								case "text", "input_text":
									toolText.WriteString(content.GetText())
								case "image":
									if info != nil {
										info.ToolResultImageCount++
										if content.Source != nil {
											info.ToolResultImageBase64Chars += len(common.Interface2String(content.Source.Data))
										}
										addToolResultMediaType(info, content)
									}
									if relocateToolResultMedia && content.Source != nil {
										imageData := fmt.Sprintf("data:%s;base64,%s", content.Source.MediaType, common.Interface2String(content.Source.Data))
										mediaMessages = append(mediaMessages, dto.MediaContent{
											Type:         "image_url",
											ImageUrl:     &dto.MessageImageUrl{Url: imageData},
											CacheControl: content.CacheControl,
										})
										mediaMoved = true
									} else {
										toolText.WriteString("[tool_result_media_omitted:image]")
										mediaFallback = true
									}
								default:
									if info != nil {
										addToolResultMediaType(info, content)
									}
									toolText.WriteString("[tool_result_media_omitted:")
									toolText.WriteString(content.Type)
									toolText.WriteString("]")
									mediaFallback = true
								}
							}
							if mediaMoved || mediaFallback {
								if toolText.Len() > 0 {
									toolText.WriteByte(' ')
								}
								if mediaMoved {
									toolText.WriteString(toolResultMediaRelocatedMarker)
								}
								if mediaMoved && mediaFallback {
									toolText.WriteByte(' ')
								}
								if mediaFallback {
									toolText.WriteString(toolResultMediaFallbackMarker)
								}
							}
							oaiToolMessage.SetStringContent(toolText.String())
							if info != nil && mediaFallback {
								info.ToolResultMediaFallback = true
							}
						}
					}
					openAIMessages = append(openAIMessages, oaiToolMessage)
				}
			}

			if len(toolCalls) > 0 {
				openAIMessage.SetToolCalls(toolCalls)
			}
			if len(mediaMessages) > 0 && len(toolCalls) == 0 {
				openAIMessage.SetMediaContent(mediaMessages)
			}
		}
		if len(openAIMessage.ParseContent()) > 0 || len(openAIMessage.ToolCalls) > 0 {
			openAIMessages = append(openAIMessages, openAIMessage)
		}
	}
	if info != nil && len(info.ToolResultMediaTypes) > 1 {
		sort.Strings(info.ToolResultMediaTypes)
	}

	openAIRequest.Messages = openAIMessages
	return &openAIRequest, nil
}

func addToolResultMediaType(info *relaycommon.RelayInfo, content dto.ClaudeMediaMessage) {
	if info == nil {
		return
	}
	mediaType := content.Type
	if content.Source != nil && content.Source.MediaType != "" {
		mediaType = content.Source.MediaType
	}
	for _, existing := range info.ToolResultMediaTypes {
		if existing == mediaType {
			return
		}
	}
	info.ToolResultMediaTypes = append(info.ToolResultMediaTypes, mediaType)
}

func requestToJSONString(v interface{}) string {
	b, err := common.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}
