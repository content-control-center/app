// Package embedopts centralises the per-request options passed to the Gemini
// Embedding 2 model (CON-101). It is a leaf package importing only the genai
// SDK so every embed call site — the document-side flows (embed_asset,
// process_pdf) and the query-side consumers (content_plan, post_assistant) —
// can share the same config without import cycles.
package embedopts

import "google.golang.org/genai"

// Dimensions is the embedding output dimensionality requested on every call.
// It is set once at boot from config.EmbedDimensions and must match the
// assets_chunks.embedding halfvec(N) column. Defaults to Gemini's native 3072.
var Dimensions int32 = 3072

// Document returns the embed config for content that is stored and later
// searched against (chunks of assets/PDFs). RETRIEVAL_DOCUMENT tells Gemini to
// optimise the vector for the corpus side of a retrieval task.
func Document() *genai.EmbedContentConfig {
	return &genai.EmbedContentConfig{
		TaskType:             "RETRIEVAL_DOCUMENT",
		OutputDimensionality: genai.Ptr(Dimensions),
		// Chunks are kept well under Gemini's 8192-token input limit, but
		// truncate rather than error on the rare oversized chunk.
		AutoTruncate: true,
	}
}

// Query returns the embed config for a search query embedded at request time
// (content-plan ranking, the assistant's searchAssetChunks tool).
// RETRIEVAL_QUERY optimises the vector for the query side of the same task.
func Query() *genai.EmbedContentConfig {
	return &genai.EmbedContentConfig{
		TaskType:             "RETRIEVAL_QUERY",
		OutputDimensionality: genai.Ptr(Dimensions),
		AutoTruncate:         true,
	}
}
