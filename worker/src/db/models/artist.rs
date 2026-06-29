use sea_orm::entity::prelude::*;

#[derive(Clone, Debug, PartialEq, DeriveEntityModel)]
#[sea_orm(table_name = "artists")]
pub struct Model {
    #[sea_orm(primary_key)]
    pub id: i64,
    #[sea_orm(unique)]
    pub name: String,
    pub created_at: DateTimeWithTimeZone,
}

#[derive(Copy, Clone, Debug, EnumIter, DeriveRelation)]
pub enum Relation {
    #[sea_orm(has_many = "super::song_artist::Entity")]
    SongArtist,
}

impl Related<super::song_artist::Entity> for Entity {
    fn to() -> RelationDef {
        Relation::SongArtist.def()
    }
}

impl ActiveModelBehavior for ActiveModel {}
