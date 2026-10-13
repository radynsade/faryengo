# To do

## Generic

- [ ] Workspace-scoped roles and permissions.
- [ ] Validate domain entities, VOs, aggregates, etc on create right in the domain layer. Don't worry about cases when domain layer object created without a constructor, because it is a convention to use them.
- [ ] Add Errors() method to domain objects. Instead of returning the first error of validation, it must validate all possible errors and return them. See `newRole` in the application layer.

## Office

- [ ] Replace form messages with alerts from the top. There is an example in the "Ant Design".
- [ ] Separate section to control workspace members, roles and permissions. Don't do per permission access, prepare prebuilt roles.
- [ ] 404 page.
- [ ] 5XX page.

## Admin panel
