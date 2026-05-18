package pipeline

// pricePer1M is a small, manually-maintained table of provider list prices in
// USD per 1M tokens, for the default models we ship. Used purely to print a
// rough cost estimate next to the usage totals. When a model is missing we
// leave the estimate at 0; the printed report says "unknown model".
type pricePer1M struct{ input, output float64 }

var modelPrices = map[string]pricePer1M{
	// OpenAI / OpenRouter (OpenAI-compatible passthrough uses the same id)
	"gpt-4o-mini":            {0.15, 0.60},
	"openai/gpt-4o-mini":     {0.15, 0.60},
	"gpt-4o":                 {2.50, 10.00},
	"openai/gpt-4o":          {2.50, 10.00},
	"gpt-4.1-mini":           {0.40, 1.60},
	"openai/gpt-4.1-mini":    {0.40, 1.60},
	// Anthropic
	"claude-haiku-4-5":       {1.00, 5.00},
	"claude-sonnet-4-6":      {3.00, 15.00},
	"claude-opus-4-7":        {15.00, 75.00},
	// Gemini
	"gemini-2.5-flash":       {0.30, 2.50},
	"gemini-2.5-flash-lite":  {0.10, 0.40},
	"gemini-2.5-pro":         {1.25, 10.00},
}

func estimateUSD(model string, inputTokens, outputTokens int) (float64, bool) {
	p, ok := modelPrices[model]
	if !ok {
		return 0, false
	}
	return (float64(inputTokens)*p.input + float64(outputTokens)*p.output) / 1_000_000, true
}
