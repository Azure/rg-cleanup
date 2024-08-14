param uami string
param uami_client_id string
param image string
param logic_app_name string = 'rg-cleaner'
param container_group string = 'rg-cleaner-cg'
param container_name string = 'rg-cleaner-cn'
param aci string = 'aci'
param dryrun bool = true
param ttl string = ''
param regex string = ''
param role_assignments bool = true
param csubscription string = subscription().subscriptionId
param location string = resourceGroup().location
param rg_name string = resourceGroup().name

var default_container_cmd = [
  'rg-cleanup.sh'
  '--identity'
]
var dryrun_cmd = [
  '--dry-run'
]
var ttl_cmd = [
  '--ttl'
  ttl
]
var regex_cmd = [
  '--regex'
  regex
]
var role_assignments_cmd = [
  '--role-assignments'
]
var add_regex_cmd = concat(default_container_cmd, empty(regex) ? [] : regex_cmd)
var add_ttl_cmd = concat(add_regex_cmd, empty(ttl) ? [] : ttl_cmd)
var container_command = concat(add_ttl_cmd, dryrun ? dryrun_cmd : [], role_assignments ? role_assignments_cmd : [])

var encoded_sub = uriComponent(csubscription)
var encoded_rg = uriComponent(rg_name)
var encoded_cg = uriComponent(container_group)
var encoded_cn = uriComponent(container_name)

var rg_external_id = '/subscriptions/${csubscription}/resourceGroups/${rg_name}'
var uami_external_id = '${rg_external_id}/providers/Microsoft.ManagedIdentity/userAssignedIdentities/${uami}'
var rg_id = '/subscriptions/${encoded_sub}/resourceGroups/${encoded_rg}'
var cg_id = '${rg_id}/providers/Microsoft.ContainerInstance/containerGroups/${encoded_cg}'
var cn_id = '${cg_id}/containers/${encoded_cn}'
var aci_api_id = '/subscriptions/${csubscription}/providers/Microsoft.Web/locations/${location}/managedApis/aci'

resource logic_app 'Microsoft.Logic/workflows@2019-05-01' = {
  name: logic_app_name
  location: location
  identity: {
    type: 'UserAssigned'
    userAssignedIdentities: {
      '${uami_external_id}': {
      }
    }
  }
  properties: {
    state: 'Enabled'
    definition: {
      '$schema': 'https://schema.management.azure.com/providers/Microsoft.Logic/schemas/2016-06-01/workflowdefinition.json#'
      contentVersion: '1.0.0.0'
      parameters: {
        '$connections': {
          defaultValue: {
          }
          type: 'Object'
        }
      }
      triggers: {
        Recurrence: {
          recurrence: {
            frequency: 'Hour'
            interval: 1
          }
          evaluatedRecurrence: {
            frequency: 'Hour'
            interval: 1
          }
          type: 'Recurrence'
        }
      }
      actions: {
        Create_or_update_a_container_group: {
          runAfter: {
          }
          type: 'ApiConnection'
          inputs: {
            body: {
              identity: {
                type: 'UserAssigned'
                userAssignedIdentities: {
                  '${uami_external_id}': {
                  }
                }
              }
              location: location
              properties: {
                containers: [
                  {
                    name: container_name
                    properties: {
                      command: container_command
                      environmentVariables: [
                        {
                          name: 'SUBSCRIPTION_ID'
                          value: csubscription
                        }
                        {
                          name: 'AAD_CLIENT_ID'
                          value: uami_client_id
                        }
                      ]
                      image: image
                      resources: {
                        requests: {
                          cpu: 1
                          memoryInGB: 1
                        }
                      }
                    }
                  }
                ]
                osType: 'Linux'
                restartPolicy: 'Never'
                sku: 'Standard'
              }
            }
            host: {
              connection: {
                name: '@parameters(\'$connections\')[\'aci\'][\'connectionId\']'
              }
            }
            method: 'put'
            path: cg_id
            queries: {
              'x-ms-api-version': '2019-12-01'
            }
          }
        }
        Delete_a_container_group: {
          runAfter: {
            Get_logs_from_a_container_instance: [
              'Succeeded'
              'TimedOut'
              'Failed'
              'Skipped'
            ]
          }
          type: 'ApiConnection'
          inputs: {
            host: {
              connection: {
                name: '@parameters(\'$connections\')[\'aci\'][\'connectionId\']'
              }
            }
            method: 'delete'
            path: cg_id
            queries: {
              'x-ms-api-version': '2019-12-01'
            }
          }
        }
        Get_logs_from_a_container_instance: {
          runAfter: {
            Until: [
              'Succeeded'
            ]
          }
          type: 'ApiConnection'
          inputs: {
            host: {
              connection: {
                name: '@parameters(\'$connections\')[\'aci\'][\'connectionId\']'
              }
            }
            method: 'get'
            path: '${cn_id}/logs'
            queries: {
              'x-ms-api-version': '2019-12-01'
            }
          }
        }
        Get_properties_of_a_container_group: {
          runAfter: {
            Create_or_update_a_container_group: [
              'Succeeded'
            ]
          }
          type: 'ApiConnection'
          inputs: {
            host: {
              connection: {
                name: '@parameters(\'$connections\')[\'aci\'][\'connectionId\']'
              }
            }
            method: 'get'
            path: cg_id
            queries: {
              'x-ms-api-version': '2019-12-01'
            }
          }
        }
        Initialize_variable: {
          runAfter: {
            Get_properties_of_a_container_group: [
              'Succeeded'
            ]
          }
          type: 'InitializeVariable'
          inputs: {
            variables: [
              {
                name: 'complete'
                type: 'string'
                value: '@body(\'Get_properties_of_a_container_group\')?[\'properties\']?[\'instanceView\']?[\'state\']'
              }
            ]
          }
        }
        Until: {
          actions: {
            Get_properties_of_a_container_group_loop: {
              type: 'ApiConnection'
              inputs: {
                host: {
                  connection: {
                    name: '@parameters(\'$connections\')[\'aci\'][\'connectionId\']'
                  }
                }
                method: 'get'
                path: cg_id
                queries: {
                  'x-ms-api-version': '2019-12-01'
                }
              }
            }
            // this works because there is only one container in the group, otherwise it might overwrite the variable
            For_each: {
              foreach: '@body(\'Get_properties_of_a_container_group_loop\')[\'properties\'][\'containers\']'
              actions: {
                Set_variable: {
                  type: 'SetVariable'
                  inputs: {
                    name: 'complete'
                    value: '@items(\'For_each\')?[\'properties\']?[\'instanceView\']?[\'currentState\']?[\'state\']'
                  }
                }
              }
              runAfter: {
                Get_properties_of_a_container_group_loop: [
                  'Succeeded'
                ]
              }
              type: 'Foreach'
            }
            Delay: {
              runAfter: {
                For_each: [
                  'Succeeded'
                ]
              }
              type: 'Wait'
              inputs: {
                interval: {
                  count: 1
                  unit: 'Minute'
                }
              }
            }
          }
          runAfter: {
            Initialize_variable: [
              'Succeeded'
            ]
          }
          expression: '@equals(variables(\'complete\'),\'Terminated\')'
          limit: {
            count: 60
            timeout: 'PT1H'
          }
          type: 'Until'
        }
      }
      outputs: {
      }
    }
    parameters: {
      '$connections': {
        value: {
          aci: {
            connectionId: apiConnection.id
            connectionName: apiConnection.name
            connectionProperties: {
              authentication: {
                identity: uami_external_id
                type: 'ManagedServiceIdentity'
              }
            }
            id: aci_api_id
          }
        }
      }
    }
  }
}

resource apiConnection 'Microsoft.Web/connections@2018-07-01-preview' = {
  name: aci
  kind: 'V1'
  location: location
  properties:{
    alternativeParameterValues: {}
    parameterValueType: 'Alternative'
    displayName: aci
    statuses: [
      {
        status: 'Ready'
      }
    ]
    api:{
      id: aci_api_id
      displayName: aci
      type: 'Microsoft.Web/locations/managedApis'
    }
  }
}
