#!/usr/bin/env bash
set -e

# ===== 构建并推送镜像到 stgharbor（离线 amd64）=====
docker login stgharbor.liangdawang.com:1180 -u stgadmin -p 3UPBnRmi3RaV

gitTag=$(git rev-parse --short HEAD)
buildDate=$(date +%Y%m%d%H%M%S)
imageTag="${buildDate}-${gitTag}"
imageName="stgharbor.liangdawang.com:1180/stg/ldw-new-api:$imageTag"

# 必须指定 aliyun_env/Dockerfile，否则会用默认 Dockerfile（联网版，离线环境拉不到基础镜像）
docker build -f aliyun_env/Dockerfile --platform linux/amd64 -t $imageName .
docker push $imageName
docker rmi $imageName


# ===== 部署到阿里云 K8s（kubectl 在远程跳板机执行）=====
NAMESPACE="server"                     # 命名空间
DEPLOYMENT_NAME="ldw-new-api"          # Deployment 名称（与 aliyun_env/stg.yaml 一致）
REMOTE="root@10.37.101.11"             # 跳板机，kubectl 在这台上

echo "开始部署 Kubernetes 服务: $DEPLOYMENT_NAME in namespace $NAMESPACE"

# 把 aliyun_env/stg.yaml 里的占位镜像替换成本次构建的真实 tag（避免占位 latest 拉不到镜像）
sed -i "s#stgharbor.liangdawang.com:1180/stg/ldw-new-api:.*#stgharbor.liangdawang.com:1180/stg/ldw-new-api:${imageTag}#g" aliyun_env/stg.yaml

# 清单在 Jenkins 工作区，远程机器上没有，先拷过去再 apply
scp aliyun_env/stg.yaml ${REMOTE}:/tmp/ldw-new-api.yaml

# 应用清单：首次创建 Deployment/Service，已存在则滚动更新；apply 的就是最新镜像
ssh -tt ${REMOTE} << EOF
/usr/bin/kubectl apply -f /tmp/ldw-new-api.yaml -n ${NAMESPACE}
/usr/bin/kubectl get deployments --namespace=${NAMESPACE}
/usr/bin/kubectl rollout status deployment/${DEPLOYMENT_NAME} -n ${NAMESPACE}
exit
EOF

echo "部署完成！当前运行的镜像为: $imageName"
exit
