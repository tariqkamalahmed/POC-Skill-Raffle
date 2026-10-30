library 'jenkins-shared-lib@master'

pipeline {
   agent {
      kubernetes {
         yamlFile 'agents/agents.yaml'
         defaultContainer 'agent'
      }
   }

   parameters {
      booleanParam(name: 'BUILD', defaultValue: true, description: 'If set to true, the build stages will run.')        
      booleanParam(name: 'LINT', defaultValue: true, description: 'If set to true, the lint stages will run.')
      booleanParam(name: 'TEST', defaultValue: true, description: 'If set to true, the test stages will run.')
      booleanParam(name: 'SCAN', defaultValue: true, description: 'If set to true, the scan stages will run.')
   }

   environment {
      IMAGE_NAME = ''
      IMAGE_REPO = '633050264585.dkr.ecr.eu-west-2.amazonaws.com'
      CROSS_ACCOUNT_ROLE_ARN = ''
      PROJECT_CODE = 'ip3a'
      AWS_ACCOUNT_SHORT_NAME = ''
      //SEMVER_TOOLS_DEBUG = "1"
   }

   stages {
      stage('Go setup') {
         steps {
            container('go') {
               sh script:'''
                #!/bin/bash
                cd $WORKSPACE
                go mod download
                make tools
                '''
            }
         }
      }

      stage('Run Go vet') {
         when { expression { return params.LINT; } }
         steps {
            container('go') {

               sh script:'''
                #!/bin/bash
                cd $WORKSPACE
                go vet -json ./... 2> vetwarnings.json
                '''
               }
               
               recordIssues enabledForFailure: true, aggregatingResults: true, tool: goVet(pattern: 'vetwarnings.json')
            }
      }

      stage('Run Go staticcheck') {
         when { expression { return params.LINT; } }
         steps {
            container('go') {
               script{
                  try {
                     sh script:'''
                      #!/bin/bash
                      cd $WORKSPACE
                      staticcheck -checks all ./...
                      '''
                  }
                  catch (err) {
                     unstable(message: "staticcheck has not passed all checks - Marking as unstable...")
                  }
               }
            }
         }
      }


      stage('Run Tests') {
         when { expression { return params.TEST; } }
         steps {
            container('go') {
               sh script:'''
               #!/bin/bash
               cd $WORKSPACE
               make test-jenkins 
               '''
            }
         }
         post {
            always {
               container('build') {
                  script {
                     helper2.withAwsCredentials(roleArn: "${}") {
                        dir("${pwd()}") {
                           sh 'cat go_report.xml | strings > report.xml'
                           junit 'report.xml'
                        }

                     }
                  }      
               }
            }
         }
      }

      stage('Run Go vulncheck') {
         when { expression { return params.SCAN; } }
         steps {
            container('go') {
               sh script:'''
                #!/bin/bash
                cd $WORKSPACE
                govulncheck ./...
                '''
            }
         }      
      }


      stage('Build Go - latest') {
         when {
            allOf {
               expression { return params.BUILD == true }
               branch 'main'
            }
         }
         steps {
            container('kaniko') {
               script {
                  helper2.withKanikoEcrLogin(configFile: '/kaniko/.docker/config.json') {
                     helper2.buildImage(
                        repository: env.IMAGE_REPO,
                        image_name: env.IMAGE_NAME,
                        image_version: "${env.BRANCH_NAME}",
                        context: '.',
                        tag_latest: true,
                     )
                  }
               }
            }
         }
      }

   }
}
