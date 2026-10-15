package templates

import (
	"github.com/robertguss/go-islands-starter/internal/db"
	"strings"
)

func initials(name string) string {
	var result []rune
	for _, word := range strings.Fields(name) {
		result = append(result, []rune(word)[0])
		if len(result) == 2 {
			break
		}
	}
	return string(result)
}
func role(u db.AppUser) string {
	if u.Reviewer {
		return "Reviewer"
	}
	if u.Requester {
		return "Requester"
	}
	if u.AccessAdmin {
		return "Access administrator"
	}
	return "Your shared workspace"
}
func statusLabel(s string) string {
	return map[string]string{"pending": "Pending", "approved": "Approved", "rejected": "Declined", "complete": "Ready", "failed": "Failed", "expired": "Expired"}[s]
}
func actionLabel(s string) string {
	return map[string]string{"request.created": "Request submitted", "request.approved": "Request approved", "request.rejected": "Request declined", "export.requested": "Export requested", "export.completed": "Export ready", "export.failed": "Export failed", "user.updated": "Access updated"}[s]
}

var previewAccounts = []struct{ ID, Name, Role string }{
	{"00000000-0000-4000-8000-000000000001", "Alex Morgan", "Requester · Own requests"},
	{"00000000-0000-4000-8000-000000000002", "Blair Chen", "Requester · Own requests"},
	{"00000000-0000-4000-8000-000000000003", "Casey Rivera", "Reviewer + requester · Shared inbox"},
	{"00000000-0000-4000-8000-000000000004", "Dana Patel", "Access administrator · People only"},
}
