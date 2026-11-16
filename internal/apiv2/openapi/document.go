// Package openapi holds the static OpenAPI 3.0 document for the v2 API.
// The Hub serves it unchanged at GET /api/v2/openapi.json.
package openapi

// Document joins the domain-owned path contracts into one static document.
// Each section preserves the documented endpoint order and response bytes.
const Document = v2OpenAPIDocumentHeader +
	v2OpenAPIAssetPaths +
	v2OpenAPIOperationPaths +
	v2OpenAPIContainerAndTransferPaths +
	v2OpenAPIAlertIncidentPaths +
	v2OpenAPIConnectorPaths +
	v2OpenAPIAccessAdministrationPaths +
	v2OpenAPIServiceNotificationPaths +
	v2OpenAPITopologyPaths +
	v2OpenAPIOperatorSettingsPaths
