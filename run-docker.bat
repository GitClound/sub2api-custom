@echo off
cd /d E:\Study\00000Git\AI\sub2api-main\deploy
echo Starting Sub2API...
docker-compose up -d
echo.
echo === Docker Containers ===
docker ps -a
echo.
echo === Docker Images ===
docker images
