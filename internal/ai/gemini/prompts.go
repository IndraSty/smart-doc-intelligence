package gemini

import "github.com/IndraSty/smart-doc-intelligence/internal/domain"

// systemPrompt is prepended to every Gemini request to set consistent
// behavior across all document types.
const systemPrompt = `You are an intelligent document analysis AI.
Your job is to analyze documents and extract structured information.
Always respond with valid JSON only. Do not include markdown code fences,
explanations, or any text outside the JSON object.
Be precise and extract only information that is explicitly present in the document.
If a field is not found, use null for its value.`

// classificationPrompt instructs Gemini to identify the document type
// before we run the type-specific extraction prompt.
const classificationPrompt = `Analyze this document and classify it.

Respond with this exact JSON format:
{
  "document_type": "<one of: invoice, contract, identity, financial, receipt, other>",
  "confidence": <float between 0.0 and 1.0>,
  "reasoning": "<one sentence explaining why you chose this type>"
}

Document types:
- invoice: bills, purchase orders, payment requests from vendors
- contract: legal agreements, terms of service, NDAs, employment contracts
- identity: ID cards, passports, driver licenses, KTP
- financial: financial reports, balance sheets, income statements, bank statements
- receipt: sales receipts, proof of purchase, transaction records
- other: anything that does not fit the above categories`

// summaryPrompt generates a 2-3 sentence summary of any document type.
const summaryPrompt = `Based on the document content and extracted information,
write a concise summary in 2-3 sentences.

Respond with this exact JSON format:
{
  "summary": "<2-3 sentence summary of the document>"
}`

// extractionPrompts maps each document type to its specific extraction prompt.
// Each prompt is tuned to extract the most relevant fields for that document type.
var extractionPrompts = map[domain.DocumentType]string{
	domain.TypeInvoice: `Extract all relevant fields from this invoice document.

Respond with this exact JSON format:
{
  "fields": [
    {"key": "invoice_number",  "value": "<string or null>",         "confidence": <0.0-1.0>},
    {"key": "vendor_name",     "value": "<string or null>",         "confidence": <0.0-1.0>},
    {"key": "vendor_address",  "value": "<string or null>",         "confidence": <0.0-1.0>},
    {"key": "customer_name",   "value": "<string or null>",         "confidence": <0.0-1.0>},
    {"key": "invoice_date",    "value": "<YYYY-MM-DD or null>",     "confidence": <0.0-1.0>},
    {"key": "due_date",        "value": "<YYYY-MM-DD or null>",     "confidence": <0.0-1.0>},
    {"key": "subtotal",        "value": <number or null>,           "confidence": <0.0-1.0>},
    {"key": "tax_amount",      "value": <number or null>,           "confidence": <0.0-1.0>},
    {"key": "total_amount",    "value": <number or null>,           "confidence": <0.0-1.0>},
    {"key": "currency",        "value": "<ISO 4217 code or null>",  "confidence": <0.0-1.0>},
    {"key": "payment_terms",   "value": "<string or null>",         "confidence": <0.0-1.0>},
    {"key": "line_items",      "value": [
      {
        "description": "<string>",
        "quantity": <number>,
        "unit_price": <number>,
        "total": <number>
      }
    ],                                                               "confidence": <0.0-1.0>},
    {"key": "notes",           "value": "<string or null>",         "confidence": <0.0-1.0>}
  ]
}`,

	domain.TypeContract: `Extract all relevant fields from this contract or legal agreement.

Respond with this exact JSON format:
{
  "fields": [
    {"key": "contract_title",     "value": "<string or null>",     "confidence": <0.0-1.0>},
    {"key": "parties_involved",   "value": ["<party1>", "<party2>"], "confidence": <0.0-1.0>},
    {"key": "effective_date",     "value": "<YYYY-MM-DD or null>", "confidence": <0.0-1.0>},
    {"key": "expiry_date",        "value": "<YYYY-MM-DD or null>", "confidence": <0.0-1.0>},
    {"key": "contract_value",     "value": <number or null>,       "confidence": <0.0-1.0>},
    {"key": "currency",           "value": "<ISO 4217 or null>",   "confidence": <0.0-1.0>},
    {"key": "governing_law",      "value": "<string or null>",     "confidence": <0.0-1.0>},
    {"key": "jurisdiction",       "value": "<string or null>",     "confidence": <0.0-1.0>},
    {"key": "key_obligations",    "value": ["<obligation1>", "<obligation2>"], "confidence": <0.0-1.0>},
    {"key": "termination_clause", "value": "<string or null>",     "confidence": <0.0-1.0>},
    {"key": "penalty_clause",     "value": "<string or null>",     "confidence": <0.0-1.0>},
    {"key": "renewal_terms",      "value": "<string or null>",     "confidence": <0.0-1.0>},
    {"key": "confidentiality",    "value": <true/false or null>,   "confidence": <0.0-1.0>},
    {"key": "non_compete",        "value": <true/false or null>,   "confidence": <0.0-1.0>}
  ]
}`,

	domain.TypeIdentity: `Extract all relevant fields from this identity document (ID card, passport, KTP, driver license).

Respond with this exact JSON format:
{
  "fields": [
    {"key": "full_name",        "value": "<string or null>",     "confidence": <0.0-1.0>},
    {"key": "id_number",        "value": "<string or null>",     "confidence": <0.0-1.0>},
    {"key": "document_type",    "value": "<KTP/Passport/SIM/other or null>", "confidence": <0.0-1.0>},
    {"key": "nationality",      "value": "<string or null>",     "confidence": <0.0-1.0>},
    {"key": "date_of_birth",    "value": "<YYYY-MM-DD or null>", "confidence": <0.0-1.0>},
    {"key": "place_of_birth",   "value": "<string or null>",     "confidence": <0.0-1.0>},
    {"key": "gender",           "value": "<Male/Female or null>","confidence": <0.0-1.0>},
    {"key": "address",          "value": "<string or null>",     "confidence": <0.0-1.0>},
    {"key": "issue_date",       "value": "<YYYY-MM-DD or null>", "confidence": <0.0-1.0>},
    {"key": "expiry_date",      "value": "<YYYY-MM-DD or null>", "confidence": <0.0-1.0>},
    {"key": "issuing_authority", "value": "<string or null>",    "confidence": <0.0-1.0>},
    {"key": "blood_type",       "value": "<string or null>",     "confidence": <0.0-1.0>},
    {"key": "marital_status",   "value": "<string or null>",     "confidence": <0.0-1.0>},
    {"key": "occupation",       "value": "<string or null>",     "confidence": <0.0-1.0>}
  ]
}`,

	domain.TypeFinancial: `Extract all relevant fields from this financial report or statement.

Respond with this exact JSON format:
{
  "fields": [
    {"key": "company_name",     "value": "<string or null>",     "confidence": <0.0-1.0>},
    {"key": "report_type",      "value": "<Balance Sheet/Income Statement/Cash Flow/other or null>", "confidence": <0.0-1.0>},
    {"key": "period_start",     "value": "<YYYY-MM-DD or null>", "confidence": <0.0-1.0>},
    {"key": "period_end",       "value": "<YYYY-MM-DD or null>", "confidence": <0.0-1.0>},
    {"key": "currency",         "value": "<ISO 4217 or null>",   "confidence": <0.0-1.0>},
    {"key": "total_revenue",    "value": <number or null>,       "confidence": <0.0-1.0>},
    {"key": "cost_of_goods",    "value": <number or null>,       "confidence": <0.0-1.0>},
    {"key": "gross_profit",     "value": <number or null>,       "confidence": <0.0-1.0>},
    {"key": "operating_expenses","value": <number or null>,      "confidence": <0.0-1.0>},
    {"key": "operating_income", "value": <number or null>,       "confidence": <0.0-1.0>},
    {"key": "net_profit",       "value": <number or null>,       "confidence": <0.0-1.0>},
    {"key": "total_assets",     "value": <number or null>,       "confidence": <0.0-1.0>},
    {"key": "total_liabilities","value": <number or null>,       "confidence": <0.0-1.0>},
    {"key": "total_equity",     "value": <number or null>,       "confidence": <0.0-1.0>},
    {"key": "earnings_per_share","value": <number or null>,      "confidence": <0.0-1.0>},
    {"key": "auditor",          "value": "<string or null>",     "confidence": <0.0-1.0>},
    {"key": "audit_opinion",    "value": "<string or null>",     "confidence": <0.0-1.0>}
  ]
}`,

	domain.TypeReceipt: `Extract all relevant fields from this receipt or proof of purchase.

Respond with this exact JSON format:
{
  "fields": [
    {"key": "merchant_name",    "value": "<string or null>",     "confidence": <0.0-1.0>},
    {"key": "merchant_address", "value": "<string or null>",     "confidence": <0.0-1.0>},
    {"key": "transaction_date", "value": "<YYYY-MM-DD or null>", "confidence": <0.0-1.0>},
    {"key": "transaction_time", "value": "<HH:MM or null>",      "confidence": <0.0-1.0>},
    {"key": "receipt_number",   "value": "<string or null>",     "confidence": <0.0-1.0>},
    {"key": "items_purchased",  "value": [
      {
        "name": "<string>",
        "quantity": <number>,
        "unit_price": <number>,
        "total": <number>
      }
    ],                                                            "confidence": <0.0-1.0>},
    {"key": "subtotal",         "value": <number or null>,       "confidence": <0.0-1.0>},
    {"key": "tax_amount",       "value": <number or null>,       "confidence": <0.0-1.0>},
    {"key": "discount_amount",  "value": <number or null>,       "confidence": <0.0-1.0>},
    {"key": "total_amount",     "value": <number or null>,       "confidence": <0.0-1.0>},
    {"key": "currency",         "value": "<ISO 4217 or null>",   "confidence": <0.0-1.0>},
    {"key": "payment_method",   "value": "<Cash/Credit/Debit/Transfer/other or null>", "confidence": <0.0-1.0>},
    {"key": "cashier_name",     "value": "<string or null>",     "confidence": <0.0-1.0>}
  ]
}`,

	domain.TypeOther: `Extract any meaningful structured information from this document.

Respond with this exact JSON format:
{
  "fields": [
    {"key": "<descriptive_field_name>", "value": "<extracted value or null>", "confidence": <0.0-1.0>}
  ]
}

Extract as many meaningful fields as you can identify.
Use snake_case for field keys. Be thorough but only include
fields that have clear values in the document.`,
}

// GetExtractionPrompt returns the prompt template for a given document type.
// Falls back to the "other" prompt if the type is not recognized.
func GetExtractionPrompt(docType domain.DocumentType) string {
	prompt, ok := extractionPrompts[docType]
	if !ok {
		return extractionPrompts[domain.TypeOther]
	}
	return prompt
}
