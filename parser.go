package http_template

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"
	"text/template"

	http "github.com/bogdanfinn/fhttp"
)

// ParseHeaderTemplate processes a template string with given data and parses it
// into an http.Header object from the bogdanfinn/fhttp library.
//
// The input template string is expected to have:
// 1. A request line (e.g., "GET / HTTP/2"), which is ignored for header construction.
// 2. Subsequent lines as "key: value" pairs.
//
// Pseudo-headers (keys starting with ':') will have their keys (e.g., ":method")
// stored in the http.PHeaderOrderKey list. Their values are ignored for the main
// header map itself.
//
// Regular headers will have their keys and values stored in the http.Header map,
// with their original casing preserved. The order of regular header keys
// will be stored in the http.HeaderOrderKey list.
//
// Special case: "cookie" headers.
// The key "cookie" (case-insensitive match, but original case of the *first*
// encountered cookie header preserved in order list) will be added to the
// http.HeaderOrderKey list *only once*. The cookie key-value pair
// will NOT be added to the main http.Header map.
func ParseHeaderTemplate(
	templateStr string,
	templateData interface{},
) (http.Header, error) {
	var processedStringBuffer bytes.Buffer
	tmpl, err := template.New("httpHeaderTemplate").Parse(templateStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse template string: %w", err)
	}

	err = tmpl.Execute(&processedStringBuffer, templateData)
	if err != nil {
		return nil, fmt.Errorf("failed to execute template: %w", err)
	}

	processedStr := processedStringBuffer.String()
	outputHeaders := make(http.Header)
	var pseudoHeaderOrder []string
	var regularHeaderOrder []string
	cookieHeaderAddedToOrder := false // Flag to track if "cookie" is already in regularHeaderOrder

	scanner := bufio.NewScanner(strings.NewReader(processedStr))

	// Skip the first line (expected to be the request line)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf(
				"error reading the first line: %w",
				err,
			)
		}
		return outputHeaders, nil // Empty template or only one line
	}

	for scanner.Scan() {
		line := scanner.Text()
		trimmedLine := strings.TrimSpace(line)
		if trimmedLine == "" {
			continue
		}

		var key, value string

		firstColonIdx := strings.IndexByte(trimmedLine, ':')

		if firstColonIdx == -1 {
			// No colon in the line, not a valid "key: value" format
			continue
		}

		potentialKeyPart := trimmedLine[:firstColonIdx]
		potentialValuePart := ""
		if firstColonIdx < len(trimmedLine)-1 {
			potentialValuePart = trimmedLine[firstColonIdx+1:]
		}

		if strings.TrimSpace(potentialKeyPart) == "" &&
			strings.HasPrefix(trimmedLine, ":") {
			// Pseudo-header pattern like ":method: GET"
			secondColonIdxInPVP := strings.IndexByte(potentialValuePart, ':')
			actualPseudoKeyNamePart := ""

			if secondColonIdxInPVP == -1 {
				actualPseudoKeyNamePart = strings.TrimSpace(potentialValuePart)
				value = ""
			} else {
				actualPseudoKeyNamePart = strings.TrimSpace(
					potentialValuePart[:secondColonIdxInPVP],
				)
				if secondColonIdxInPVP < len(potentialValuePart)-1 {
					value = strings.TrimSpace(
						potentialValuePart[secondColonIdxInPVP+1:],
					)
				} else {
					value = ""
				}
			}
			if actualPseudoKeyNamePart == "" {
				key = ":"
			} else {
				key = ":" + actualPseudoKeyNamePart
			}
		} else {
			// Regular header or other format
			key = strings.TrimSpace(potentialKeyPart)
			value = strings.TrimSpace(potentialValuePart)
		}

		if key == "" {
			continue
		}

		if strings.HasPrefix(key, ":") {
			// This is a pseudo-header.
			pseudoHeaderOrder = append(pseudoHeaderOrder, key)
			// Pseudo-header values are not added to the main outputHeaders map.
		} else {
			// This is a regular HTTP header.
			isCookieHeader := strings.ToLower(key) == "cookie"

			if isCookieHeader {
				if !cookieHeaderAddedToOrder {
					// Add "cookie" (with its original casing from this first encounter)
					// to regularHeaderOrder only once.
					regularHeaderOrder = append(regularHeaderOrder, key)
					cookieHeaderAddedToOrder = true
				}
				// Cookie key-value pair is NOT added to outputHeaders map.
			} else {
				// For all other regular headers (not "cookie"):
				// Add its key (preserving original case) to the regularHeaderOrder list.
				regularHeaderOrder = append(regularHeaderOrder, key)
				// Add the header to the map.
				if existingValues, ok := outputHeaders[key]; ok {
					outputHeaders[key] = append(existingValues, value)
				} else {
					outputHeaders[key] = []string{value}
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf(
			"error scanning lines from processed template: %w",
			err,
		)
	}

	if len(pseudoHeaderOrder) > 0 {
		outputHeaders[http.PHeaderOrderKey] = pseudoHeaderOrder
	}
	if len(regularHeaderOrder) > 0 {
		outputHeaders[http.HeaderOrderKey] = regularHeaderOrder
	}

	return outputHeaders, nil
}
