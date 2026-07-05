package service

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// RecordEmailSendFailure learns from a synchronous send error: a permanent
// (5xx) RCPT rejection means the submission server refused this exact address
// (invalid mailbox / provider invalid-address library), so the address is added
// to the suppression list and never mailed again. Transient errors and
// sender/content failures are ignored. Every email send path (transactional and
// marketing) should call this with the send error.
func RecordEmailSendFailure(email string, sendErr error) {
	if sendErr == nil || !common.IsPermanentRecipientRejection(sendErr) {
		return
	}
	if err := model.UpsertEmailSuppression(email, model.SuppressionReasonHardBounce, model.SuppressionSourceSMTP, sendErr.Error()); err != nil {
		common.SysError(fmt.Sprintf("failed to record email suppression for %s: %s", email, err.Error()))
		return
	}
	common.SysLog(fmt.Sprintf("email suppression: %s hard-bounced at RCPT (5xx), added to suppression list", email))
}
