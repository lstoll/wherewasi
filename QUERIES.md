### Checkins with a person

```
select count(*) from checkins c
join checkin_people cp on (c.id = cp.checkin_id)
join people p on (cp.person_id = p.id)
where p.name = '<Person Name>';
```

### Checkins with a person at a given location

```
select count(*) from checkins c
join checkin_people cp on (c.id = cp.checkin_id)
join people p on (cp.person_id = p.id)
join venues v on (c.venue_id = v.id)
where p.name = '<Person Name>' and
      v.name = '<Location Name>';
```

### Checkins with a person within a given distance of a named venue

```
.load mod_spatialite

select v.name, c.checkin_time from venues v
join checkins c on (c.venue_id = v.id)
join checkin_people cp on (c.id = cp.checkin_id)
join people p on (cp.person_id = p.id)
where PtDistWithin(
        (select MakePoint(lat, lng, 4326) from venues where name = '<Source Venue>'),
        MakePoint(v.lat, v.lng, 4326), 500) -- last field is distance in metres
    and p.name = '<Person Name>'
order by c.checkin_time asc;
```