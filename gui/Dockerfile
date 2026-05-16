FROM python:3.12-slim

WORKDIR /app

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY app.py .

ENV GATEWAY_URL=http://api-gateway:8000 \
    GATEWAY_API_KEY=""

EXPOSE 8501

CMD ["streamlit", "run", "app.py", \
    "--server.port=8501", \
    "--server.address=0.0.0.0", \
    "--server.headless=true", \
    "--browser.gatherUsageStats=false"]