// Package carddatatest holds small bulk data fixtures in the shape Scryfall
// writes them, for tests that build a local card store
package carddatatest

// Oracle is an oracle_cards file, which marks the printing Scryfall picks for
// each card
const Oracle = `{"id":"bolt-clu","oracle_id":"o-bolt","name":"Lightning Bolt"}
{"id":"stp-frc","oracle_id":"o-stp","name":"Swords to Plowshares"}
`

// Cards is a default_cards file with eight printings a store keeps and one art
// series card it leaves out
const Cards = `{"id":"bolt-2x2","oracle_id":"o-bolt","name":"Lightning Bolt","layout":"normal","set":"2x2","collector_number":"117","released_at":"2022-07-08","games":["paper"],"edhrec_rank":5,"mana_cost":"{R}","type_line":"Instant","colors":["R"],"image_uris":{"art_crop":"x"}}
{"id":"bolt-clu","oracle_id":"o-bolt","name":"Lightning Bolt","layout":"normal","set":"clu","collector_number":"141","released_at":"2024-02-23","games":["paper"],"edhrec_rank":5,"mana_cost":"{R}","type_line":"Instant","colors":["R"]}
{"id":"bolt-promo","oracle_id":"o-bolt","name":"Lightning Bolt","layout":"normal","set":"plst","collector_number":"1","released_at":"2025-01-01","promo":true,"games":["paper"],"edhrec_rank":5,"type_line":"Instant"}
{"id":"bolt-art","name":"Lightning Bolt // Lightning Bolt","layout":"art_series","set":"a2x2","collector_number":"1","released_at":"2022-07-08","games":["paper"]}
{"id":"stp-frc","oracle_id":"o-stp","name":"Swords to Plowshares","layout":"normal","set":"frc","collector_number":"37","released_at":"2025-01-01","games":["paper"],"edhrec_rank":20,"type_line":"Instant"}
{"id":"emeritus","oracle_id":"o-em","name":"Emeritus of Truce // Swords to Plowshares","layout":"modal_dfc","set":"sos","collector_number":"13","released_at":"2026-04-01","games":["paper"],"edhrec_rank":900,"type_line":"Creature // Instant","card_faces":[{"name":"Emeritus of Truce"},{"name":"Swords to Plowshares"}]}
{"id":"vault","oracle_id":"o-vault","name":"Lim-Dûl's Vault","layout":"normal","set":"c13","collector_number":"197","released_at":"2013-11-01","games":["paper"],"edhrec_rank":3000,"type_line":"Instant"}
{"id":"boltwave","oracle_id":"o-wave","name":"Boltwave","layout":"normal","set":"fdn","collector_number":"79","released_at":"2024-11-15","games":["paper"],"edhrec_rank":40,"type_line":"Sorcery"}
{"id":"rev","name":"Sol Ring // Sol Ring","layout":"reversible_card","set":"sld","collector_number":"999","released_at":"2023-01-01","games":["paper"],"card_faces":[{"name":"Sol Ring","oracle_id":"o-sol"},{"name":"Sol Ring","oracle_id":"o-sol"}]}
`
