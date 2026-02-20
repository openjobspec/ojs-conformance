package main

// assertions.go provides gRPC-specific assertion helpers for the conformance
// runner. It bridges the HTTP-centric test assertions (which use HTTP status
// codes) to the gRPC world by translating gRPC status codes to their HTTP
// equivalents.
//
// The actual assertion evaluation logic (body matchers, timing, etc.) is
// shared with the HTTP runner via the lib package. This file contains only
// the gRPC-specific parts: status code mapping and status assertion dispatch.

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/openjobspec/ojs-conformance/lib"
	"google.golang.org/grpc/codes"
)

// httpStatusToGRPCCode maps HTTP status codes to their closest gRPC
// equivalents. This is the inverse of GRPCCodeToHTTPStatus (in adapter.go)
// and is useful for diagnostic messages.
var httpStatusToGRPCCode = map[int]codes.Code{
	200: codes.OK,
	201: codes.OK, // with HTTPStatusOverride
	400: codes.InvalidArgument,
	401: codes.Unauthenticated,
	403: codes.PermissionDenied,
	404: codes.NotFound,
	409: codes.AlreadyExists,
	412: codes.FailedPrecondition,
	429: codes.ResourceExhausted,
	500: codes.Internal,
	501: codes.Unimplemented,
	503: codes.Unavailable,
	504: codes.DeadlineExceeded,
}

// HTTPStatusToGRPCCode returns the gRPC code that most closely maps to an
// HTTP status code. If no mapping exists, codes.Unknown is returned.
func HTTPStatusToGRPCCode(httpStatus int) codes.Code {
	if c, ok := httpStatusToGRPCCode[httpStatus]; ok {
		return c
	}
	return codes.Unknown
}

// evaluateStatusAssertion handles various status assertion formats against
// the HTTP-equivalent status code derived from the gRPC response:
//   - integer: exact match (e.g., 200)
//   - string: matcher like "number:range(400,422)" or "one_of:200,409"
//   - object: {"$in": [200, 409]}
func evaluateStatusAssertion(raw json.RawMessage, actual int) error {
	// Try as integer
	var statusInt int
	if err := json.Unmarshal(raw, &statusInt); err == nil {
		if actual != statusInt {
			return fmt.Errorf("Expected status %d, got %d (gRPC code: %s)",
				statusInt, actual, HTTPStatusToGRPCCode(actual))
		}
		return nil
	}

	// Try as string matcher
	var statusStr string
	if err := json.Unmarshal(raw, &statusStr); err == nil {
		if strings.HasPrefix(statusStr, "one_of:") {
			codesStr := statusStr[len("one_of:"):]
			codesList := strings.Split(codesStr, ",")
			for _, codeStr := range codesList {
				codeStr = strings.TrimSpace(codeStr)
				code, err := strconv.Atoi(codeStr)
				if err != nil {
					return fmt.Errorf("invalid status code %q in one_of matcher", codeStr)
				}
				if actual == code {
					return nil
				}
			}
			return fmt.Errorf("expected status one of [%s], got %d (gRPC code: %s)",
				codesStr, actual, HTTPStatusToGRPCCode(actual))
		}
		return lib.MatchAssertion(raw, float64(actual))
	}

	// Try as object (e.g., {"$in": [200, 409]})
	var statusObj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &statusObj); err == nil {
		if inRaw, ok := statusObj["$in"]; ok {
			var inList []int
			if err := json.Unmarshal(inRaw, &inList); err == nil {
				for _, s := range inList {
					if actual == s {
						return nil
					}
				}
				return fmt.Errorf("Expected status in %v, got %d (gRPC code: %s)",
					inList, actual, HTTPStatusToGRPCCode(actual))
			}
		}
	}

	return fmt.Errorf("Unknown status assertion format: %s", string(raw))
}
