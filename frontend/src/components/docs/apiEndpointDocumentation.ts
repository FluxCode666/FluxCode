export type ApiDocumentationMethod = 'GET' | 'POST' | 'PUT' | 'DELETE'

export interface ApiEndpointRequestParameter {
  name: string
  location: string
  required: boolean
  type: string
  description: string
  defaultValue?: string
  example?: string
}

export interface ApiEndpointResponseParameter {
  name: string
  type: string
  description: string
  example?: string
}

export interface ApiEndpointDocumentation {
  id: string
  method: ApiDocumentationMethod
  path: string
  title: string
  description: string
  requestDescription: string
  requestParameters: ApiEndpointRequestParameter[]
  responseParameters: ApiEndpointResponseParameter[]
  requestExample: string
  responseExample: string
}
