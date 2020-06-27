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

### Device locations by month

```
select count(*) as count, strftime("%m-%Y", timestamp) as 'mm-yyy'
from device_locations group by strftime("%m-%Y", timestamp)
order by timestamp asc;
```

### Categories visited in a city

```
select count(c.id) as count, json_extract(fsq_raw, '$.venue.categories[0].name') as category
from checkins c
join venues v on (c.venue_id = v.id)
where PtDistWithin(
        MakePoint(50.8503, 4.3517, 4326), -- Brussels, BE
        MakePoint(v.lat, v.lng, 4326), 20000)
group by category order by count desc;
```
