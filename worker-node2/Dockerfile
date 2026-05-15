FROM python:3.12-slim

WORKDIR /app

RUN apt-get update && apt-get install -y --no-install-recommends \
    default-libmysqlclient-dev build-essential pkg-config \
    && rm -rf /var/lib/apt/lists/*

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY app/ app/

ENV MYSQL_URL=mysql+pymysql://root:rootpass@mysql-worker2:3306/distdb \
    HMAC_SECRET=change-me-in-production \
    TOKEN_TTL_SECONDS=30 \
    NODE_ID=3 \
    PORT=8082

EXPOSE 8082

CMD ["sh", "-c", "uvicorn app.main:app --host 0.0.0.0 --port $PORT"]