import http from 'k6/http';

export default function () {
  let data = String(Math.floor(Math.random() * (10000000 - 1)) + 1);
  http.del('http://localhost:8080/key/' + data, '', {
    tags: { name: 'DELETE /key/[id]' },
    headers: { 'Content-Type': 'text/plain' }
  });
}
