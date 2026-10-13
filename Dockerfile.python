FROM python:3.12-slim
ENV PYTHONDONTWRITEBYTECODE=1 PYTHONUNBUFFERED=1 DATABASE_PATH=/app/var/assistant.sqlite3
WORKDIR /app
COPY requirements.lock ./
RUN pip install --no-cache-dir -r requirements.lock \
    && groupadd --gid 10001 rhythm \
    && useradd --uid 10001 --gid rhythm --no-create-home rhythm
COPY app ./app
COPY scripts ./scripts
COPY data ./data
COPY docs ./docs
RUN mkdir -p /app/var && chown -R rhythm:rhythm /app/var
USER rhythm
EXPOSE 8000
HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 CMD python -c "import urllib.request; urllib.request.urlopen('http://127.0.0.1:8000/health', timeout=3)"
CMD ["python", "-m", "uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8000"]
