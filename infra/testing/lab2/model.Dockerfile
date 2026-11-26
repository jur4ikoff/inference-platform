FROM python:3.13-alpine
COPY model.py /model.py
ENTRYPOINT ["python3", "/model.py"]
