package agents

func agentInstallScriptTemplate() string {
	return agentInstallScriptTemplateHeader() +
		agentInstallScriptTemplateArgsAndUninstall() +
		agentInstallScriptTemplateDockerValidationAndPreflight() +
		agentInstallScriptTemplateInstallFlow() +
		agentInstallScriptTemplateVerifyAndSummary()
}
