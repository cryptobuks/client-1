// @flow
import {onUserClick} from '../actions/profile'
import * as Creators from '../actions/chat/creators'
import * as SearchCreators from '../actions/searchv3/creators'
import * as SearchConstants from '../constants/searchv3'
import {debounce} from 'lodash'
import {compose, withState, withHandlers, defaultProps} from 'recompose'
import {connect} from 'react-redux'
import {profileSearchResultArray} from '../constants/selectors'
import Search from './search'

import type {Props} from './search'
import type {TypedState} from '../constants/reducer'

type HocIntermediateProps = {
  _clearSearchResults: () => void,
  _onClick: (id: string) => void,
  _onChangeText: (nextText: string) => void,
  _onSelectService: () => void,
  _search: (term: string, service: SearchConstants.Service) => void,
}

const mapStateToProps = (state: TypedState) => ({
  ids: profileSearchResultArray(state),
})
const mapDispatchToProps = (dispatch: Dispatch, {navigateUp, onBack, onToggleSidePanel}: Props) => ({
  _clearSearchResults: () => dispatch(Creators.clearSearchResults()),
  _onClick: username => {
    dispatch(navigateUp())
    dispatch(onUserClick(username))
  },
  _search: debounce(
    (term: string, service) => dispatch(SearchCreators.search(term, 'profile:updateSearchResults', service)),
    1e3
  ),
  onClose: () => dispatch(navigateUp()),
})

export default compose(
  connect(mapStateToProps, mapDispatchToProps),
  withState('usernameText', '_onChangeText', ''),
  withState('selectedService', '_onSelectService', 'Keybase'),
  withState('searchText', 'onChangeSearchText', ''),
  withHandlers({
    onChangeText: (props: Props & HocIntermediateProps) => nextText => {
      props.onChangeSearchText(nextText)
      props._search(nextText, props.selectedService)
    },
    onClick: (props: Props & HocIntermediateProps) => id => {
      props._onClick(id)
      props._onChangeText('')
      props._clearSearchResults()
    },
    onSelectService: (props: Props & HocIntermediateProps) => nextService => {
      props._onSelectService(nextService)
      props._search(props.searchText, nextService)
    },
  }),
  defaultProps({
    placeholder: 'Type someone',
    showAddButton: false,
    userItems: [],
  })
)(Search)
