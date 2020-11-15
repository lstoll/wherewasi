function setupMap(geoJSON) {
    var map = L.map('map-canvas');

    L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
        attribution: '&copy; <a href="http://osm.org/copyright">OpenStreetMap</a> contributors'
    }).addTo(map);

    var features = L.geoJSON(geoJSON)
    features.addTo(map);

    map.fitBounds(features.getBounds());


    // L.marker([51.5, -0.09]).addTo(map)
    //     .bindPopup("<b>Hello world!</b><br />I am a popup.").openPopup();

    // L.circle([51.508, -0.11], 500, {
    //     color: 'red',
    //     fillColor: '#f03',
    //     fillOpacity: 0.5
    // }).addTo(map).bindPopup("I am a circle.");

    // L.polygon([
    //     [51.509, -0.08],
    //     [51.503, -0.06],
    //     [51.51, -0.047]
    // ]).addTo(map).bindPopup("I am a polygon.");


    // var popup = L.popup();

    // function onMapClick(e) {
    //     popup
    //         .setLatLng(e.latlng)
    //         .setContent("You clicked the map at " + e.latlng.toString())
    //         .openOn(map);
    // }

    // map.on('click', onMapClick);

    // var recentLocs;

    // try {
    //     let response = await fetch("/recent");
    //     recentLocs = await response.json();
    // } catch (e) {
    //     console.log("fetching: " + e.message);
    // }

    // let circles = [];

    // for (var loc of recentLocs) {
    //     let circ = L.circle([loc.lat, loc.lng], { radius: loc.accuracy });
    //     circles.push(circ);
    //     circ.addTo(map);
    // }


    // not working, maybe https://github.com/Leaflet/Leaflet/issues/3280 ?
    // let group = new L.featureGroup(circles);
    // map.fitBounds(group.getBounds());

    // navigator.geolocation.getCurrentPosition(function (position) {
    //     map.setView([position.coords.latitude, position.coords.longitude], 13);
    // }, function (err) {
    //     console.log("error getting geolocation: " + err.message);
    //     map.setView([36.1627, -86.7816], 13);
    // });
}
