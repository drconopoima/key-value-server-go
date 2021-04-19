import http from 'k6/http';
export default function () {
  let data = String(Math.floor(Math.random() * (10000000 - 1)) + 1);
  http.get('http://localhost:8080/key/'+data);
}
