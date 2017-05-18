node("master") {
    def PWD = pwd()
    def project_dir = "${PWD}/src/github.com/appscode/voyager"
    stage("set env") {
        env.GOPATH = "${PWD}"
        env.GOBIN = "${GOPATH}/bin"
        env.PATH = "$env.PATH:${env.GOBIN}:/usr/local/go/bin"
        sh "mkdir -p ${env.GOBIN}"
    }
    dir("${project_dir}") {
        stage("test") {
            sh "export hh=333333 && printenv"
        }
        stage("test1") {
            sh 'echo $hh'
        }
        stage("Test3") {
            sh "printenv"
        }
        stage("checkout") {
            checkout scm
        }
        stage("builddeps") {
            sh "sudo ./hack/builddeps.sh"
        }
        stage("dependency") {
            sh "glide slow"
        }
        stage("build binary") {
            sh "./hack/make.py"
        }
        stage("build docker") {
            sh "./hack/docker/voyager/setup.sh"
        }
    }
}
