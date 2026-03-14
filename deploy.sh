#!/bin/bash
set -euo pipefail

AWS_ACCOUNT_ID="678990962884"
AWS_REGION="us-east-1"
ECR_REPO="ziptax-mcp"
ECS_CLUSTER="ziptax-mcp"
ECS_SERVICE="ziptax-mcp"
IMAGE_URI="${AWS_ACCOUNT_ID}.dkr.ecr.${AWS_REGION}.amazonaws.com/${ECR_REPO}"

echo "==> Logging in to ECR..."
aws ecr get-login-password --region "${AWS_REGION}" | \
  docker login --username AWS --password-stdin "${AWS_ACCOUNT_ID}.dkr.ecr.${AWS_REGION}.amazonaws.com"

echo "==> Building Docker image (linux/amd64)..."
docker build --platform linux/amd64 -t "${IMAGE_URI}:latest" .

echo "==> Pushing image to ECR..."
docker push "${IMAGE_URI}:latest"

echo "==> Deploying to ECS..."
aws ecs update-service \
  --cluster "${ECS_CLUSTER}" \
  --service "${ECS_SERVICE}" \
  --force-new-deployment \
  --region "${AWS_REGION}" \
  --query 'service.deployments[0].{Status:status,Running:runningCount,Desired:desiredCount}' \
  --output table

echo "==> Deploy initiated. Waiting for service stability..."
aws ecs wait services-stable \
  --cluster "${ECS_CLUSTER}" \
  --services "${ECS_SERVICE}" \
  --region "${AWS_REGION}"

echo "==> Deploy complete! Service is stable."
