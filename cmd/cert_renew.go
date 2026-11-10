package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func newCertRenewCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "renew",
		Short: t("经本机续期通道，为已托管的域名向 CA 续期", "renew one ACME-managed certificate through the local channel"),
		Long: t(
			"把续期请求交给正在运行的 fast-https。监听与 GET /runtime 分开，只接受 127.0.0.1，并校验状态目录里的令牌。\n"+
				"域名必须在 ACME 托管名单里。必须同时给出 --yes、--domain 和 --request-id。\n"+
				"成功时只打印 obtained、written、reloaded。失败不替换现有证书，输出里没有私钥和 PEM。",
			"ask the running fast-https to renew one certificate. The listener is separate from GET /runtime,\n"+
				"accepts only 127.0.0.1, and checks the token in the state directory.\n"+
				"The domain must be in the ACME plan. --yes, --domain and --request-id are all required.\n"+
				"Success prints only obtained, written and reloaded. Failure leaves the current certificate in place.\n"+
				"The output contains no private key and no PEM.",
		),
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE:         runCertRenew,
	}
	c.Flags().String("domain", "", t("要续期的域名", "domain to renew"))
	c.Flags().Bool("yes", false, t("确认向 CA 发起续期", "confirm the renewal"))
	return c
}

func runCertRenew(cmd *cobra.Command, args []string) error {
	const commandID = "cert.renew"
	yes, _ := cmd.Flags().GetBool("yes")
	if !yes {
		return usageError(fmt.Errorf("cert renew requires --yes"))
	}
	domain, _ := cmd.Flags().GetString("domain")
	domain = strings.TrimSpace(domain)
	if domain == "" {
		return usageError(fmt.Errorf("cert renew requires --domain"))
	}
	rid := strings.TrimSpace(requestIDOf(cmd))
	if rid == "" {
		return usageError(fmt.Errorf("cert renew requires --request-id"))
	}
	token, err := readRenewToken()
	if err != nil {
		return &CLIError{Command: commandID, Code: CodeServiceDown, Message: "renew token is not available", Cause: err}
	}
	payload, err := json.Marshal(renewRequest{Domain: domain, RequestID: rid})
	if err != nil {
		return &CLIError{Command: commandID, Code: CodeInternal, Message: "renew request could not be encoded", Cause: err}
	}
	httpReq, err := http.NewRequest(http.MethodPost, renewEndpointURL(), bytes.NewReader(payload))
	if err != nil {
		return &CLIError{Command: commandID, Code: CodeInternal, Message: "renew request could not be built", Cause: err}
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+token)
	client := &http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Do(httpReq)
	if err != nil {
		return &CLIError{Command: commandID, Code: CodeServiceDown, Message: "renew endpoint is unreachable", Cause: err}
	}
	defer resp.Body.Close()
	var stages renewStages
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&stages); err != nil {
		return &CLIError{Command: commandID, Code: CodeOperationFailed, Message: "renew endpoint returned an unreadable response", Cause: err}
	}
	if resp.StatusCode != http.StatusOK {
		return &CLIError{Command: commandID, Code: renewStatusCode(resp.StatusCode), Message: renewStatusMessage(resp.StatusCode)}
	}
	data := map[string]any{
		"domain":   domain,
		"obtained": stages.Obtained,
		"written":  stages.Written,
		"reloaded": stages.Reloaded,
	}
	if jsonMode(cmd) {
		return emitOK(cmd, commandID, data, nil)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "domain:   %s\nobtained: %t\nwritten:  %t\nreloaded: %t\n",
		domain, stages.Obtained, stages.Written, stages.Reloaded)
	return nil
}

func renewEndpointURL() string {
	if v := strings.TrimSpace(os.Getenv("FASTHTTPS_RENEW_URL")); v != "" {
		return v
	}
	return "http://" + renewListenAddr + "/renew"
}

func renewStatusCode(status int) string {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return CodePermissionDenied
	case http.StatusNotFound:
		return CodeNotFound
	case http.StatusConflict:
		return CodeConflict
	default:
		return CodeOperationFailed
	}
}

func renewStatusMessage(status int) string {
	switch status {
	case http.StatusUnauthorized:
		return "renew token was rejected"
	case http.StatusForbidden:
		return "renew request was not from loopback"
	case http.StatusNotFound:
		return "domain is not managed by ACME"
	case http.StatusConflict:
		return "renew request is already in progress"
	default:
		return "renew failed"
	}
}
