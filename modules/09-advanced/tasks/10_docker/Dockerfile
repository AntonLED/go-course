# ЗАГОТОВКА: так делать НЕ надо. Исправьте этот файл по требованиям из README.
# Сборка из КОРНЯ репозитория (там go.mod):
#   docker build -f modules/09-advanced/tasks/10_docker/Dockerfile -t course-server .
FROM golang:latest
WORKDIR /src
COPY . .
RUN go build -o /server ./modules/09-advanced/tasks/10_docker/cmd/server
EXPOSE 8080
CMD /server
