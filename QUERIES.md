# Checkins with a person

```
select count(*) from checkins c
join checkin_people cp on (c.id = cp.checkin_id)
join people p on (cp.person_id = p.id)
where p.name = '<Person Name>';
```
