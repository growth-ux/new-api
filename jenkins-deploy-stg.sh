#!/usr/bin/env bash
set -e

printf '%s' "$SWR_PASSWORD" | docker login --username "$SWR_USERNAME" --password-stdin swr.cofco-cn-beijing-1.cofco.com

gitTag=$(git rev-parse --short HEAD)
buildDate=$(date +%Y%m%d%H%M%S)
imageTag="${buildDate}-${gitTag}"
imageName="swr.cofco-cn-beijing-1.cofco.com/liangdawang-test-liangdawang/stg/new-api:$imageTag"
docker build --platform linux/arm64 -f DockerfileStg -t "$imageName" .
docker push $imageName
docker rmi $imageName


# 定义 K8s 变量（与 stg.yaml 保持一致）
NAMESPACE="server"                     # 命名空间
DEPLOYMENT_NAME="new-api-stg"          # Deployment 名称
CONTAINER_NAME="new-api"               # container 名称

echo "开始更新 Kubernetes 服务: $DEPLOYMENT_NAME in namespace $NAMESPACE"

# 更新镜像版本
kubectl set image deployment/${DEPLOYMENT_NAME} ${CONTAINER_NAME}=${imageName} -n ${NAMESPACE}

# 监控滚动更新状态（等待 Pod 启动完成，失败则 Jenkins 红灯）
kubectl rollout status deployment/${DEPLOYMENT_NAME} -n ${NAMESPACE}

echo "部署完成！当前运行的镜像为: $imageName"
exit
