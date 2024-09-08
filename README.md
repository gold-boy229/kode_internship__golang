CURL - добавление записи

curl -X POST http://localhost:3000/add-note -H "Content-Type: application/json" -d '{"user_id":1,"content":"New note by frst user"}' -u One:password
curl -X POST http://localhost:3000/add-note -H "Content-Type: application/json" -d '{"user_id":2,"content":"helo wrld!  whee yoou go?"}' -u Two:password



CURL - вывод списка записей пользователя
curl -v -H "Accept: application/json" http://localhost:3000/notes?user_id=1 -u One:password
curl -v -H "Accept: application/json" http://localhost:3000/notes?user_id=2 -u Two:password


CURL - тест спеллера
curl -X GET https://speller.yandex.net/services/spellservice.json/checkText?text=liffe

curl -X POST "https://speller.yandex.net/services/spellservice.json/checkText" -H "Content-Type: application/x-www-form-urlencoded" -d "text=Hello, wrld!"


