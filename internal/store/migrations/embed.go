// Package migrations embarque les migrations goose du schéma SQLite de Watchoo.
//
// Les fichiers sont numérotés (00001_init.sql, 00002_….sql) et annotés « -- +goose Up » /
// « -- +goose Down ». Ils servent aussi de schéma à sqlc (voir sqlc.yaml à la racine du dépôt).
// Une migration publiée n'est jamais modifiée : toute évolution passe par un nouveau fichier.
package migrations

import "embed"

// FS contient les fichiers *.sql de migration, à la racine du système de fichiers
// (ex. « 00001_init.sql »). Il est passé tel quel à goose.NewProvider.
//
//go:embed *.sql
var FS embed.FS
