import http from 'k6/http';

const SERVER_ADDR = __ENV.SERVER_ADDR || 'localhost:8080';

export default function () {
  let data = String(Math.floor(Math.random() * (10000000 - 1)) + 1);
  http.post('http://' + SERVER_ADDR + '/key/' + data, data, {
    tags: { name: 'POST /key/[id]' },
    headers: { 'Content-Type': 'text/plain' }
  });
}
