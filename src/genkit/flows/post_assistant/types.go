package post_assistant

import (
	"github.com/firebase/genkit/go/ai"

	"github.com/content-control-center/app/src/repository"
)

// PostAssistantRequest is the input to the postAssistant flow.
type PostAssistantRequest struct {
	PostID      string `json:"postId"`
	Instruction string `json:"instruction"`
}

// PostAssistantResponse is the structured output from the assistant.
type PostAssistantResponse struct {
	Action             string `json:"action"                       jsonschema:"description=edited when content was changed or declined when the request is out of scope,enum=edited,enum=declined"`
	UpdatedDescription string `json:"updatedDescription"           jsonschema:"description=The full updated post description; empty when action is declined"`
	Explanation        string `json:"explanation"                  jsonschema:"description=Human-readable explanation of what was changed or why the request was declined"`
	SaveVersion        bool   `json:"saveVersion"                  jsonschema:"description=True when a new version snapshot should be created"`
	VersionNote        string `json:"versionNote,omitempty"        jsonschema:"description=Short note describing the version; only present when saveVersion is true"`
}

// PostAssistantRepos bundles all repository dependencies for the flow.
type PostAssistantRepos struct {
	Posts    repository.PostRepository
	Assets   repository.AssetRepository
	Chunks   repository.AssetChunksRepository
	Campaigns repository.CampaignRepository
	Versions repository.PostVersionRepository
	Messages repository.PostAssistantMessageRepository
}

// PostAssistantFlowConfig holds settings for the post assistant flow.
type PostAssistantFlowConfig struct {
	ModelID         string
	MaxOutputTokens int64
	Embedder        ai.Embedder // nil = semantic search unavailable
}

// ValidationError is returned when preconditions are not met (HTTP 400).
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

// AIError is returned when the model call fails (HTTP 502).
type AIError struct{ Msg string }

func (e *AIError) Error() string { return e.Msg }
